package service

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const (
	serviceAccountDir           = "/var/run/secrets/kubernetes.io/serviceaccount"
	serviceAccountTokenPath     = serviceAccountDir + "/token"
	serviceAccountNamespacePath = serviceAccountDir + "/namespace"
	serviceAccountCAPath        = serviceAccountDir + "/ca.crt"
)

type KubernetesClient struct {
	BaseURL    string
	Token      string
	Namespace  string
	HTTPClient *http.Client
}

func (c KubernetesClient) Apply(ctx context.Context, manifest []byte) error {
	var job kubernetesJob
	if err := json.Unmarshal(manifest, &job); err != nil {
		return fmt.Errorf("decode Job manifest: %w", err)
	}
	if job.Kind != "Job" {
		return fmt.Errorf("manifest kind %q is not supported", job.Kind)
	}
	namespace, err := c.namespace(job.Metadata.Namespace)
	if err != nil {
		return err
	}
	resp, err := c.request(ctx, http.MethodPost, "/apis/batch/v1/namespaces/"+url.PathEscape(namespace)+"/jobs", manifest)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return kubernetesResponseError(resp)
	}
	return nil
}

func (c KubernetesClient) ReadReport(ctx context.Context, namespace, jobName string) ([]byte, error) {
	namespace, err := c.namespace(namespace)
	if err != nil {
		return nil, err
	}
	status, err := c.ReadJobStatus(ctx, namespace, jobName)
	if err != nil {
		return nil, err
	}
	if status.Status != "succeeded" {
		if status.Error != "" {
			return nil, fmt.Errorf("job %s is %s: %s", jobName, status.Status, status.Error)
		}
		return nil, fmt.Errorf("job %s is %s", jobName, status.Status)
	}
	pod, err := c.firstPodForJob(ctx, namespace, jobName)
	if err != nil {
		return nil, err
	}
	logPath := "/api/v1/namespaces/" + url.PathEscape(namespace) + "/pods/" + url.PathEscape(pod) + "/log"
	query := url.Values{"container": []string{"reviewer"}}
	resp, err := c.request(ctx, http.MethodGet, logPath+"?"+query.Encode(), nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	data, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return nil, readErr
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("read pod logs: %s: %s", resp.Status, strings.TrimSpace(string(data)))
	}
	return data, nil
}

func (c KubernetesClient) ReadJobStatus(ctx context.Context, namespace, jobName string) (JobRuntimeStatus, error) {
	namespace, err := c.namespace(namespace)
	if err != nil {
		return JobRuntimeStatus{}, err
	}
	resp, err := c.request(ctx, http.MethodGet, "/apis/batch/v1/namespaces/"+url.PathEscape(namespace)+"/jobs/"+url.PathEscape(jobName), nil)
	if err != nil {
		return JobRuntimeStatus{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	data, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return JobRuntimeStatus{}, readErr
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return JobRuntimeStatus{}, kubernetesStatusError(resp.Status, data)
	}
	var job kubernetesJob
	if err := json.Unmarshal(data, &job); err != nil {
		return JobRuntimeStatus{}, fmt.Errorf("decode Job status: %w", err)
	}
	for _, condition := range job.Status.Conditions {
		if condition.Status != "True" {
			continue
		}
		switch condition.Type {
		case "Complete":
			return JobRuntimeStatus{Status: "succeeded"}, nil
		case "Failed":
			return JobRuntimeStatus{Status: "failed", Error: firstNonEmpty(condition.Message, condition.Reason)}, nil
		}
	}
	if job.Status.Failed > 0 && job.Status.Active == 0 {
		return JobRuntimeStatus{Status: "failed"}, nil
	}
	if job.Status.Succeeded > 0 {
		return JobRuntimeStatus{Status: "succeeded"}, nil
	}
	return JobRuntimeStatus{Status: "running"}, nil
}

func (c KubernetesClient) firstPodForJob(ctx context.Context, namespace, jobName string) (string, error) {
	query := url.Values{"labelSelector": []string{"job-name=" + jobName}}
	resp, err := c.request(ctx, http.MethodGet, "/api/v1/namespaces/"+url.PathEscape(namespace)+"/pods?"+query.Encode(), nil)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	data, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return "", readErr
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", kubernetesStatusError(resp.Status, data)
	}
	var pods kubernetesPodList
	if err := json.Unmarshal(data, &pods); err != nil {
		return "", fmt.Errorf("decode pod list: %w", err)
	}
	for _, pod := range pods.Items {
		if pod.Metadata.Name != "" {
			return pod.Metadata.Name, nil
		}
	}
	return "", fmt.Errorf("no pod found for job %s", jobName)
}

func (c KubernetesClient) request(ctx context.Context, method, resourcePath string, body []byte) (*http.Response, error) {
	baseURL, err := c.baseURL()
	if err != nil {
		return nil, err
	}
	token, err := c.token()
	if err != nil {
		return nil, err
	}
	client, err := c.httpClient()
	if err != nil {
		return nil, err
	}
	u := strings.TrimRight(baseURL, "/") + "/" + strings.TrimLeft(resourcePath, "/")
	req, err := http.NewRequestWithContext(ctx, method, u, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return client.Do(req)
}

func (c KubernetesClient) baseURL() (string, error) {
	if c.BaseURL != "" {
		return strings.TrimRight(c.BaseURL, "/"), nil
	}
	host := os.Getenv("KUBERNETES_SERVICE_HOST")
	port := os.Getenv("KUBERNETES_SERVICE_PORT")
	if host == "" || port == "" {
		return "", fmt.Errorf("kubernetes service host/port are not set")
	}
	return "https://" + host + ":" + port, nil
}

func (c KubernetesClient) namespace(explicit string) (string, error) {
	if explicit != "" {
		return explicit, nil
	}
	if c.Namespace != "" {
		return c.Namespace, nil
	}
	data, err := os.ReadFile(serviceAccountNamespacePath)
	if err != nil {
		return "", fmt.Errorf("read service account namespace: %w", err)
	}
	namespace := strings.TrimSpace(string(data))
	if namespace == "" {
		return "", fmt.Errorf("service account namespace is empty")
	}
	return namespace, nil
}

func (c KubernetesClient) token() (string, error) {
	if c.Token != "" {
		return c.Token, nil
	}
	data, err := os.ReadFile(serviceAccountTokenPath)
	if err != nil {
		return "", fmt.Errorf("read service account token: %w", err)
	}
	token := strings.TrimSpace(string(data))
	if token == "" {
		return "", fmt.Errorf("service account token is empty")
	}
	return token, nil
}

func (c KubernetesClient) httpClient() (*http.Client, error) {
	if c.HTTPClient != nil {
		return c.HTTPClient, nil
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if data, err := os.ReadFile(serviceAccountCAPath); err == nil {
		pool := x509.NewCertPool()
		if ok := pool.AppendCertsFromPEM(data); !ok {
			return nil, fmt.Errorf("load Kubernetes service account CA")
		}
		transport.TLSClientConfig = &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}
	}
	return &http.Client{Transport: transport, Timeout: 30 * time.Second}, nil
}

func kubernetesResponseError(resp *http.Response) error {
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	return kubernetesStatusError(resp.Status, data)
}

func kubernetesStatusError(status string, data []byte) error {
	var body struct {
		Message string `json:"message"`
		Reason  string `json:"reason"`
	}
	_ = json.Unmarshal(data, &body)
	detail := strings.TrimSpace(firstNonEmpty(body.Message, body.Reason, string(data)))
	if detail == "" {
		return fmt.Errorf("kubernetes API returned %s", status)
	}
	return fmt.Errorf("kubernetes API returned %s: %s", status, detail)
}

type kubernetesJob struct {
	Kind     string `json:"kind"`
	Metadata struct {
		Name      string `json:"name"`
		Namespace string `json:"namespace"`
	} `json:"metadata"`
	Status struct {
		Active     int `json:"active"`
		Succeeded  int `json:"succeeded"`
		Failed     int `json:"failed"`
		Conditions []struct {
			Type    string `json:"type"`
			Status  string `json:"status"`
			Reason  string `json:"reason"`
			Message string `json:"message"`
		} `json:"conditions"`
	} `json:"status"`
}

type kubernetesPodList struct {
	Items []struct {
		Metadata struct {
			Name string `json:"name"`
		} `json:"metadata"`
	} `json:"items"`
}
