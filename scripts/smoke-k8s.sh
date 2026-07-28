#!/usr/bin/env sh
set -eu

ROOT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
KUBE_CONTEXT="${KUBE_CONTEXT:-kind-${KIND_CLUSTER:-codex-reviewer-e2e}}"
NAMESPACE="${NAMESPACE:-codex-reviewer-e2e}"
RUNNER_IMAGE="${RUNNER_IMAGE:-codex-reviewer:phase1}"
JOB_NAME="${SMOKE_K8S_JOB_NAME:-codex-reviewer-smoke}"
CONFIGMAP_NAME="${SMOKE_K8S_CONFIGMAP_NAME:-codex-reviewer-smoke-checks}"
TIMEOUT="${SMOKE_K8S_TIMEOUT:-180s}"

cd "$ROOT_DIR"

kubectl --context "$KUBE_CONTEXT" create namespace "$NAMESPACE" --dry-run=client -o yaml | kubectl --context "$KUBE_CONTEXT" apply -f -
kubectl --context "$KUBE_CONTEXT" -n "$NAMESPACE" create configmap "$CONFIGMAP_NAME" \
  --from-file=smoke-checks.sh=scripts/smoke-checks.sh \
  --dry-run=client -o yaml | kubectl --context "$KUBE_CONTEXT" apply -f -

kubectl --context "$KUBE_CONTEXT" -n "$NAMESPACE" delete job "$JOB_NAME" --ignore-not-found=true --wait=true

cat <<EOF | kubectl --context "$KUBE_CONTEXT" -n "$NAMESPACE" apply -f -
apiVersion: batch/v1
kind: Job
metadata:
  name: ${JOB_NAME}
spec:
  backoffLimit: 0
  ttlSecondsAfterFinished: 300
  template:
    spec:
      restartPolicy: Never
      containers:
        - name: smoke
          image: ${RUNNER_IMAGE}
          imagePullPolicy: Never
          command: ["sh", "/smoke/smoke-checks.sh", "codex-reviewer"]
          volumeMounts:
            - name: smoke-checks
              mountPath: /smoke
              readOnly: true
      volumes:
        - name: smoke-checks
          configMap:
            name: ${CONFIGMAP_NAME}
EOF

if ! kubectl --context "$KUBE_CONTEXT" -n "$NAMESPACE" wait --for=condition=complete --timeout="$TIMEOUT" "job/$JOB_NAME"; then
  kubectl --context "$KUBE_CONTEXT" -n "$NAMESPACE" describe "job/$JOB_NAME" >&2 || true
  kubectl --context "$KUBE_CONTEXT" -n "$NAMESPACE" logs "job/$JOB_NAME" -c smoke >&2 || true
  exit 1
fi

kubectl --context "$KUBE_CONTEXT" -n "$NAMESPACE" logs "job/$JOB_NAME" -c smoke
