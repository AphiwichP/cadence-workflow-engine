# Cadence Workflow Engine — Runbook (Task #8)

> **คนอ่าน:** SRE/DevOps ที่ต้องการ deploy Cadence Workflow Engine บน GKE ใหม่ตั้งแต่ต้น
> **เวลาที่ใช้:** ~1 ชั่วโมง
> **ความยาก:** Hard

---

## สารบัญ

1. [Prerequisites](#prerequisites)
2. [Architecture Overview](#architecture-overview)
3. [Step 8.1 — Deploy Cadence Server via Helm](#step-81--deploy-cadence-server-via-helm)
4. [Step 8.2 — Verify Cadence Web UI](#step-82--verify-cadence-web-ui)
5. [Step 8.3 — Register Domain via CLI](#step-83--register-domain-via-cli)
6. [Step 8.4 — Build & Deploy Go Worker](#step-84--build--deploy-go-worker)
7. [Step 8.5 — Monitoring + Alerting](#step-85--monitoring--alerting)
8. [ทดสอบ End-to-End](#ทดสอบ-end-to-end)
9. [Quick Reference](#quick-reference)

---

## Prerequisites

### Cluster & Tools ที่ต้องมี

| Tool | Version | ตรวจสอบ |
|------|---------|---------|
| `kubectl` | 1.19+ | `kubectl version --client` |
| `helm` | 3.2+ | `helm version` |
| `gcloud` | latest | `gcloud version` |
| `docker` | latest | `docker --version` |
| `go` | 1.22+ | `go version` |

### GKE Cluster

```bash
# Scale up cluster ก่อนเริ่ม (ถ้า scale down อยู่)
gcloud container clusters resize sre-lab-cluster \
  --node-pool=default-pool \
  --num-nodes=3 \
  --zone=asia-southeast1-c

# ตั้ง credentials
gcloud container clusters get-credentials sre-lab-cluster \
  --zone=asia-southeast1-c \
  --project=sre-lab-493007

# ตรวจ nodes พร้อม
kubectl get nodes
```

<!-- [รูป: kubectl get nodes แสดง 3 nodes STATUS=Ready] -->

---

## Architecture Overview

```
┌─────────────────────────────────────────────────────────────┐
│  GKE Cluster — namespace: ai-platform                       │
│                                                             │
│  ┌──────────────┐    ┌────────────────────────────────┐     │
│  │   Cadence    │    │       Cadence Services         │     │
│  │  Cassandra   │◄───│  frontend :7933/:7833          │     │
│  │  (StatefulSet│    │  history  :7934/:7834          │     │
│  │   8Gi PVC)   │    │  matching :7935/:7835          │     │
│  └──────────────┘    │  worker   :7939                │     │
│                      └────────────────────────────────┘     │
│                                    ▲                         │
│  ┌──────────────┐                  │ gRPC :7833             │
│  │  cadence-    │──────────────────┘                        │
│  │  platform-   │  (Go worker — poll for tasks)             │
│  │  worker      │                                           │
│  └──────────────┘                                           │
│                                                             │
│  ┌──────────────┐                                           │
│  │  cadence-web │  Web UI :8088                             │
│  └──────────────┘                                           │
└─────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────┐
│  namespace: monitoring                                       │
│  Prometheus → scrape :9090 ← Cadence services              │
│  Grafana    → Dashboard ID 10373 (Cadence Frontend)         │
│  AlertManager → rules: backlog>100, timeout_rate>5%        │
└─────────────────────────────────────────────────────────────┘
```

---

## Step 8.1 — Deploy Cadence Server via Helm

### 1. เพิ่ม Helm repository

```bash
helm repo add cadence https://cadence-workflow.github.io/cadence-charts
helm repo update
helm search repo cadence
```

ผลที่ได้:
```
NAME             CHART VERSION   APP VERSION   DESCRIPTION
cadence/cadence  1.1.0           v1.3.6        Cadence is a distributed...
```

### 2. สร้าง values.yaml

ไฟล์: [values.yaml](values.yaml)

```yaml
# ── Frontend service ──────────────────────────────────────────
frontend:
  replicas: 1
  resources:
    requests:
      cpu: "100m"
      memory: "256Mi"
    limits:
      cpu: "1000m"
      memory: "2Gi"

# ── History service ───────────────────────────────────────────
history:
  replicas: 1
  resources:
    requests:
      cpu: "100m"
      memory: "256Mi"
    limits:
      cpu: "1000m"
      memory: "2Gi"

# ── Matching service ──────────────────────────────────────────
matching:
  replicas: 1
  resources:
    requests:
      cpu: "100m"
      memory: "256Mi"
    limits:
      cpu: "1000m"
      memory: "2Gi"

# ── Worker service ────────────────────────────────────────────
worker:
  replicas: 1
  resources:
    requests:
      cpu: "100m"
      memory: "256Mi"
    limits:
      cpu: "1000m"
      memory: "2Gi"

# ── Server config ─────────────────────────────────────────────
config:
  persistence:
    numHistoryShards: 4
    defaultStore: "default"
    visibilityStore: "visibility"
    advancedVisibilityStore: "visibility"   # ไม่ใช้ Elasticsearch

    database:
      driver: "cassandra"
      cassandra:
        hosts: "cadence-cassandra.ai-platform.svc.cluster.local"
        port: 9042
        keyspace: "cadence"
        visibilityKeyspace: "cadence_visibility"
        replicationFactor: 1
        user: "cassandra"
        password: "cassandra"
        protoVersion: 4
        maxConns: 10
      elasticsearch:
        enabled: false

# ── Metrics + ServiceMonitor ──────────────────────────────────
metrics:
  enabled: true
  port: 9090
  serviceMonitor:
    enabled: true
    namespace: "monitoring"
    scrapeInterval: 15s

# ── Dynamic config ────────────────────────────────────────────
dynamicConfig:
  values:
    system.minRetentionDays:
      - value: 0
        constraints: {}
    system.writeVisibilityStoreName:
      - value: "db"
    system.readVisibilityStoreName:
      - value: "db"

# ── Schema jobs ───────────────────────────────────────────────
schema:
  serverJob:
    enabled: true
    resources:
      requests:
        cpu: "100m"
        memory: "128Mi"
      limits:
        cpu: "500m"
        memory: "512Mi"
  elasticSearchJob:
    enabled: false

# ── Disable unused sub-charts ─────────────────────────────────
elasticsearch:
  enabled: false

kafka:
  enabled: false

# ── Bundled Cassandra ─────────────────────────────────────────
cassandra:
  enabled: true
  replicaCount: 1
  dbUser:
    user: cassandra
    password: "cassandra"
  cluster:
    name: cassandra
    seedCount: 1
  resources:
    requests:
      cpu: "100m"
      memory: "512Mi"
    limits:
      cpu: "1000m"
      memory: "2Gi"
  persistence:
    enabled: true
    storageClass: "standard-rwo"
    accessModes:
      - ReadWriteOnce
    size: 8Gi
```

> ⚠️ **สำคัญ:** ต้อง set `elasticsearch.enabled: false` และ `kafka.enabled: false` ไม่งั้น chart จะ deploy sub-chart พวกนั้นมาด้วย (9+ pods เพิ่ม)

### 3. Deploy

```bash
# Dry-run ก่อนเสมอ
helm install cadence cadence/cadence \
  -n ai-platform \
  -f values.yaml \
  --dry-run

# Deploy จริง
helm install cadence cadence/cadence \
  -n ai-platform \
  -f values.yaml
```

### 4. รอ pods พร้อม

```bash
kubectl get pods -n ai-platform -l "app.kubernetes.io/instance=cadence" --watch
```

<!-- [รูป: kubectl get pods แสดง cadence-cassandra-0, cadence-frontend, history, matching, worker, web ทั้งหมด STATUS=Running] -->

ผลที่ต้องได้:

| Pod | Status |
|-----|--------|
| `cadence-cassandra-0` | Running |
| `cadence-frontend-*` | Running |
| `cadence-history-*` | Running |
| `cadence-matching-*` | Running |
| `cadence-worker-*` | Running |
| `cadence-web-*` | Running |
| `cadence-schema-server-*` | Completed |

> ℹ️ frontend และ matching อาจ restart 1 ครั้งตอน start — เป็นเรื่องปกติ self-heal เองครับ

### 5. ตรวจ services

```bash
kubectl get svc -n ai-platform -l "app.kubernetes.io/instance=cadence"
```

---

## Step 8.2 — Verify Cadence Web UI

### Port-forward

รันใน **terminal แยก**:

```bash
kubectl port-forward svc/cadence-web 8088:8088 -n ai-platform
```

เปิด browser: `http://localhost:8088`

<!-- [รูป: Cadence Web UI แสดง All domains มี cadence-batcher และ cadence-system] -->

> ✅ ถ้าเห็นหน้า "All domains" พร้อม `cadence-batcher` และ `cadence-system` = Web UI ทำงานถูกต้อง

---

## Step 8.3 — Register Domain via CLI

### หาชื่อ frontend pod

```bash
kubectl get pods -n ai-platform -l "app.kubernetes.io/component=frontend"
```

### Register domain

```bash
kubectl exec -it <frontend-pod-name> \
  -n ai-platform -- \
  cadence --address localhost:7933 \
  --do platform-workflows \
  domain register \
  --rd 3 \
  --desc "Platform workflows"
```

ผลที่ได้: `Domain platform-workflows successfully registered.`

### Verify

```bash
kubectl exec -it <frontend-pod-name> \
  -n ai-platform -- \
  cadence --address localhost:7933 \
  --do platform-workflows \
  domain describe
```

<!-- [รูป: output domain describe แสดง Status: REGISTERED, RetentionInDays: 3] -->

ตรวจสอบ:
- `Status: REGISTERED` ✓
- `RetentionInDays: 3` ✓
- `ActiveClusterName: cluster0` ✓

---

## Step 8.4 — Build & Deploy Go Worker

### โครงสร้างไฟล์

```
cadence-worker/
├── main.go          ← worker setup + เชื่อมต่อ Cadence frontend
├── workflow.go      ← PlatformWorkflow definition
├── activity.go      ← HTTPCallActivity + DBWriteActivity
├── go.mod
├── Dockerfile
└── k8s/
    └── deployment.yaml
```

### 1. เตรียม Go module

```bash
cd cadence-worker
go mod tidy
go build ./...   # ตรวจ compile error ก่อน
```

### 2. ติดตั้ง Docker (WSL2)

ถ้ายังไม่มี Docker ใน WSL2:

```bash
sudo apt-get update && sudo apt-get install -y \
  ca-certificates curl gnupg lsb-release

sudo install -m 0755 -d /etc/apt/keyrings
curl -fsSL https://download.docker.com/linux/ubuntu/gpg | \
  sudo gpg --dearmor -o /etc/apt/keyrings/docker.gpg

echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] \
  https://download.docker.com/linux/ubuntu $(lsb_release -cs) stable" | \
  sudo tee /etc/apt/sources.list.d/docker.list > /dev/null

sudo apt-get update && sudo apt-get install -y docker-ce docker-ce-cli containerd.io
sudo service docker start
sudo usermod -aG docker $USER && newgrp docker
```

### 3. สร้าง Artifact Registry repository

```bash
gcloud artifacts repositories create cadence \
  --repository-format=docker \
  --location=asia-southeast1 \
  --project=sre-lab-493007
```

### 4. Build Docker image

```bash
docker build -t asia-southeast1-docker.pkg.dev/sre-lab-493007/cadence/cadence-platform-worker:latest \
  cadence-worker/
```

### 5. Login และ Push

```bash
# แก้ไข Docker config ให้ใช้ token แทน credential helper
# (จำเป็นเฉพาะ WSL2 ที่ gcloud ติดตั้งบน Windows)
cat > ~/.docker/config.json << 'EOF'
{
  "auths": {
    "https://index.docker.io/v1/": {}
  }
}
EOF

# Login ด้วย access token
gcloud auth print-access-token | \
  docker login -u oauth2accesstoken --password-stdin \
  https://asia-southeast1-docker.pkg.dev

# Push
docker push asia-southeast1-docker.pkg.dev/sre-lab-493007/cadence/cadence-platform-worker:latest
```

### 6. Grant IAM permission ให้ GKE node ดึง image ได้

```bash
# หา Compute Engine service account
gcloud iam service-accounts list \
  --project=sre-lab-493007 \
  --filter="displayName:Compute Engine default" \
  --format="value(email)"

# Grant artifactregistry.reader
gcloud artifacts repositories add-iam-policy-binding cadence \
  --location=asia-southeast1 \
  --project=sre-lab-493007 \
  --member="serviceAccount:<compute-sa-email>" \
  --role="roles/artifactregistry.reader"
```

> ⚠️ ถ้าไม่ grant permission นี้ GKE node จะ pull image ไม่ได้ (403 Forbidden)

### 7. Deploy

```bash
kubectl apply -f cadence-worker/k8s/deployment.yaml
kubectl get pods -n ai-platform -l app=cadence-platform-worker
```

<!-- [รูป: pod cadence-platform-worker STATUS=Running] -->

### 8. ตรวจ logs

```bash
kubectl logs -n ai-platform -l app=cadence-platform-worker
```

ผลที่ต้องเห็น:
```json
{"msg":"Started Workflow Worker","Domain":"platform-workflows","TaskList":"platform-task-list"}
{"msg":"Started Activity Worker","Domain":"platform-workflows","TaskList":"platform-task-list"}
```

---

## Step 8.5 — Monitoring + Alerting

### 1. สร้าง monitoring namespace และ deploy Prometheus stack

ไฟล์: [prometheus-values.yaml](prometheus-values.yaml)

```bash
kubectl create namespace monitoring

helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
helm repo update

helm install prometheus prometheus-community/kube-prometheus-stack \
  -n monitoring \
  -f prometheus-values.yaml
```

รอ pods พร้อม:
```bash
kubectl get pods -n monitoring --watch
```

### 2. Upgrade Cadence เพื่อเปิด ServiceMonitor

```bash
helm upgrade cadence cadence/cadence \
  -n ai-platform \
  -f values.yaml
```

### 3. Apply alert rules

ไฟล์: [cadence-alerts.yaml](cadence-alerts.yaml)

```bash
kubectl apply -f cadence-alerts.yaml
```

ตรวจสอบ:
```bash
kubectl get prometheusrule -n monitoring | grep cadence
kubectl get servicemonitor -n monitoring | grep cadence
```

### 4. เข้า Grafana

รันใน terminal แยก:
```bash
kubectl port-forward svc/prometheus-grafana 3000:80 -n monitoring
```

เปิด `http://localhost:3000` — login: `admin` / `admin`

### 5. Import Cadence Dashboard

1. ไปที่ **Dashboards → New → Import**
2. ใส่ ID **`10373`** ในช่อง Grafana.com → กด **Load**
3. เลือก **Prometheus** เป็น data source
4. กด **Import**

<!-- [รูป: Grafana dashboard Cadence Frontend แสดง Request Vs Error, Frontend API Latencies, Persistence metrics] -->

---

## ทดสอบ End-to-End

### Trigger workflow ด้วย CLI

```bash
kubectl exec -it <frontend-pod> -n ai-platform -- \
  cadence --address localhost:7933 \
  --do platform-workflows \
  workflow start \
  --tl platform-task-list \
  --wt main.PlatformWorkflow \
  --et 60 \
  -i '{"URL":"https://httpbin.org/get","DBKey":"test-1","DBValue":"hello"}'
```

ผลที่ได้: `Started Workflow Id: <uuid>, run Id: <uuid>`

### ดู workflow history

```bash
kubectl exec -it <frontend-pod> -n ai-platform -- \
  cadence --address localhost:7933 \
  --do platform-workflows \
  workflow show --wid <workflow-id>
```

<!-- [รูป: workflow show แสดง WorkflowExecutionStarted → HTTPCallActivity → DBWriteActivity → WorkflowExecutionCompleted] -->

workflow ที่สำเร็จจะมี event ครบ:
1. `WorkflowExecutionStarted`
2. `ActivityTaskCompleted` (HTTPCallActivity — status=200)
3. `ActivityTaskCompleted` (DBWriteActivity)
4. `WorkflowExecutionCompleted`

---

## Quick Reference

### Port-forward

```bash
# Web UI
kubectl port-forward svc/cadence-web 8088:8088 -n ai-platform

# Grafana
kubectl port-forward svc/prometheus-grafana 3000:80 -n monitoring
```

### Endpoints ภายใน cluster

| Service | Host:Port |
|---------|----------|
| Frontend (TChannel) | `cadence-frontend.ai-platform.svc.cluster.local:7933` |
| Frontend (gRPC) | `cadence-frontend.ai-platform.svc.cluster.local:7833` |
| Web UI | `cadence-web.ai-platform.svc.cluster.local:8088` |

### Rebuild & Redeploy Worker

```bash
cd cadence-worker

docker build -t asia-southeast1-docker.pkg.dev/sre-lab-493007/cadence/cadence-platform-worker:latest .

gcloud auth print-access-token | \
  docker login -u oauth2accesstoken --password-stdin \
  https://asia-southeast1-docker.pkg.dev

docker push asia-southeast1-docker.pkg.dev/sre-lab-493007/cadence/cadence-platform-worker:latest

kubectl rollout restart deployment/cadence-platform-worker -n ai-platform
```
