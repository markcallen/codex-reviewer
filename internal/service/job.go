package service

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

type JobOptions struct {
	ReviewID              string
	Namespace             string
	ReviewerImage         string
	SidecarImage          string
	ServiceAccount        string
	OpenAISecretName      string
	OpenAISecretKey       string
	GitHubSecretName      string
	GitHubSecretKey       string
	ProxyURL              string
	ActiveDeadlineSeconds int
	TTLSeconds            int
}

func JobManifest(req ReviewRequest, opts JobOptions) ([]byte, error) {
	if opts.ReviewID == "" {
		return nil, fmt.Errorf("review id is required")
	}
	if opts.ReviewerImage == "" {
		return nil, fmt.Errorf("reviewer image is required")
	}
	if opts.SidecarImage == "" {
		return nil, fmt.Errorf("sidecar image is required")
	}
	if opts.OpenAISecretName == "" {
		return nil, fmt.Errorf("OpenAI secret name is required")
	}
	if opts.OpenAISecretKey == "" {
		opts.OpenAISecretKey = "api-key"
	}
	if opts.GitHubSecretName != "" && opts.GitHubSecretKey == "" {
		opts.GitHubSecretKey = "token"
	}
	if opts.ProxyURL == "" {
		opts.ProxyURL = "http://127.0.0.1:8888"
	}
	if opts.ActiveDeadlineSeconds == 0 {
		opts.ActiveDeadlineSeconds = 3600
	}
	if opts.TTLSeconds == 0 {
		opts.TTLSeconds = 3600
	}

	requestJSON, err := req.JSON()
	if err != nil {
		return nil, err
	}

	reviewerEnv := []any{
		map[string]string{"name": "REVIEW_ID", "value": opts.ReviewID},
		map[string]string{"name": "REVIEW_REQUEST_JSON", "value": string(requestJSON)},
		map[string]string{"name": "REVIEW_OUTPUT_DIR", "value": "/out"},
		map[string]string{"name": "HTTPS_PROXY", "value": opts.ProxyURL},
		map[string]string{"name": "HTTP_PROXY", "value": opts.ProxyURL},
		map[string]string{"name": "ALL_PROXY", "value": opts.ProxyURL},
		map[string]any{
			"name": "CODEX_API_KEY",
			"valueFrom": map[string]any{
				"secretKeyRef": map[string]string{
					"name": opts.OpenAISecretName,
					"key":  opts.OpenAISecretKey,
				},
			},
		},
	}
	if opts.GitHubSecretName != "" {
		reviewerEnv = append(reviewerEnv, map[string]any{
			"name": "GITHUB_TOKEN",
			"valueFrom": map[string]any{
				"secretKeyRef": map[string]string{
					"name": opts.GitHubSecretName,
					"key":  opts.GitHubSecretKey,
				},
			},
		})
	}

	job := map[string]any{
		"apiVersion": "batch/v1",
		"kind":       "Job",
		"metadata": map[string]any{
			"name": "codex-review-" + dnsLabel(opts.ReviewID),
		},
		"spec": map[string]any{
			"activeDeadlineSeconds":   opts.ActiveDeadlineSeconds,
			"ttlSecondsAfterFinished": opts.TTLSeconds,
			"template": map[string]any{
				"metadata": map[string]any{
					"labels": map[string]string{
						"app.kubernetes.io/name":        "codex-reviewer",
						"app.kubernetes.io/component":   "review-job",
						"codex-reviewer.openai/id":      opts.ReviewID,
						"codex-reviewer.openai/profile": req.ProfileName,
					},
				},
				"spec": map[string]any{
					"restartPolicy": "Never",
					"containers": []any{
						map[string]any{
							"name":  "reviewer",
							"image": opts.ReviewerImage,
							"args":  []string{"service", "runner"},
							"env":   reviewerEnv,
							"volumeMounts": []any{
								map[string]string{"name": "workspace", "mountPath": "/workspace"},
								map[string]string{"name": "out", "mountPath": "/out"},
							},
							"workingDir": "/workspace",
						},
						map[string]any{
							"name":  "openai-egress",
							"image": opts.SidecarImage,
							"env": []any{
								map[string]any{
									"name": "OPENAI_API_KEY",
									"valueFrom": map[string]any{
										"secretKeyRef": map[string]string{
											"name": opts.OpenAISecretName,
											"key":  opts.OpenAISecretKey,
										},
									},
								},
							},
							"ports": []any{
								map[string]any{"name": "proxy", "containerPort": 8888},
							},
						},
					},
					"volumes": []any{
						map[string]any{"name": "workspace", "emptyDir": map[string]any{}},
						map[string]any{"name": "out", "emptyDir": map[string]any{}},
					},
				},
			},
		},
	}
	if opts.Namespace != "" {
		job["metadata"].(map[string]any)["namespace"] = opts.Namespace
	}
	if opts.ServiceAccount != "" {
		job["spec"].(map[string]any)["template"].(map[string]any)["spec"].(map[string]any)["serviceAccountName"] = opts.ServiceAccount
	}

	data, err := json.MarshalIndent(job, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

var nonDNSLabel = regexp.MustCompile(`[^a-z0-9-]+`)

func dnsLabel(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = nonDNSLabel.ReplaceAllString(value, "-")
	value = strings.Trim(value, "-")
	if value == "" {
		value = "review"
	}
	if len(value) > 50 {
		value = value[:50]
		value = strings.TrimRight(value, "-")
	}
	return value
}
