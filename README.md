Here is an updated, production-ready README.md for ShadowCast. It fills in all the TODO placeholders, reflects your actual Kubernetes, Envoy, and Prometheus architecture, and incorporates the newly added Helm chart instructions.

Markdown
# ShadowCast

[![License](https://img.shields.io/badge/License-Apache_2.0-blue.svg)](https://opensource.org/licenses/Apache-2.0)
[![Kubernetes](https://img.shields.io/badge/Kubernetes-v1.28%2B-blue)](https://kubernetes.io/)
[![Helm](https://img.shields.io/badge/Helm-v3-blue)](https://helm.sh/)

An enterprise-grade traffic shadowing, mirror testing, and differential analysis framework for Kubernetes workloads using Envoy Proxy, Differencia, and Prometheus.

---

## Overview

**ShadowCast** enables zero-risk production testing by non-intrusively capturing real live traffic from primary services and mirroring it to a candidate (shadow) version or a differential analysis engine (`diferencia-service`). 

By leveraging Envoy proxy's `request_mirror_policies` alongside Prometheus monitoring and custom alerting rules, ShadowCast allows engineering teams to detect API drift, status code mismatches, and performance regressions in candidate services without impacting end-user traffic or production response latencies.

---

## Key Features

* **Zero-Impact Traffic Mirroring:** Asynchronously duplicates live production requests to shadow environments.
* **Differential Analysis Integration:** Routes mirrored payloads to `diferencia-service` to compare response parity between primary (`order-service`) and candidate (`shadow-service`) versions.
* **Prometheus Alerting & Observability:** Pre-configured `PrometheusRule` manifests detect shadow traffic drops (`ShadowTrafficDrift`) and HTTP 5xx spikes (`ShadowServiceHighErrorRate`).
* **Grafana Dashboard Included:** Built-in JSON dashboard templates for immediate traffic visibility.
* **Flexible Packaging:** Deployable via direct Kustomize manifests, raw Kubernetes YAML bundles, or a complete Helm chart.

---

## Getting Started

### Prerequisites

* **Go:** `v1.24.6+`
* **Docker:** `v17.03+`
* **kubectl:** `v1.11.3+`
* **Helm:** `v3.0+`
* **Kubernetes Cluster:** `v1.28+` (Kind, Minikube, EKS, GKE, or AKS)

---

## Deployment Options

### Option 1: Deploying via Helm (Recommended)

The easiest way to deploy ShadowCast along with its Envoy configuration, Prometheus setup, and Differencia controller is using the included Helm chart.

```bash
# 1. Lint the chart to verify template syntax
helm lint charts/shadowcast

# 2. Render templates locally (Dry-run test)
helm template shadowcast-release charts/shadowcast

# 3. Install or Upgrade the chart on your cluster
helm upgrade --install shadowcast charts/shadowcast -n default
Option 2: Deploying via Kubebuilder / Kustomize
If you are developing or customizing the controller operator directly:

Bash
# 1. Build and push your image to a personal registry:
make docker-build docker-push IMG=<your-registry>/shadowcast:v1.0.0

# 2. Install the CRDs into the cluster:
make install

# 3. Deploy the Manager to the cluster:
make deploy IMG=<your-registry>/shadowcast:v1.0.0

# 4. Apply sample CR instances:
kubectl apply -k config/samples/
Option 3: Deploying via Single-File Bundle
To release or deploy without Kustomize/Helm dependencies:

Bash
# Generate dist/install.yaml bundle
make build-installer IMG=<your-registry>/shadowcast:v1.0.0

# Apply directly
kubectl apply -f [https://raw.githubusercontent.com/](https://raw.githubusercontent.com/)<your-org>/shadowcast/main/dist/install.yaml
Verification & Health Checks
Once deployed, verify that all core deployments and service discovery endpoints are operational:

Bash
# Check running pods
kubectl get pods -n default -l "app in (envoy, diferencia, prometheus, order-service, shadow-service)"

# Check internal DNS resolution for the shadow service from within the cluster
kubectl exec -it deployment/diferencia -n default -- python3 -c "import socket; print(socket.gethostbyname('shadow-service.default.svc.cluster.local'))"

# Check active Prometheus alert rules
kubectl get prometheusrule -n default
Uninstallation
Helm Release
Bash
helm uninstall shadowcast -n default
Kustomize / CRD Cleanup
Bash
# Delete sample instances
kubectl delete -k config/samples/

# Remove controller deployment
make undeploy

# Uninstall CRDs
make uninstall
Contributing
We welcome contributions to ShadowCast! Please follow these guidelines:

Fork the Repository: Create a feature branch (git checkout -b feature/amazing-feature).

Local Testing: Ensure all code passes formatting and lint checks (make lint, helm lint charts/shadowcast).

Commit Changes: Use conventional commit messages (feat: add new Envoy filter policy).

Push & PR: Push your branch and open a Pull Request against main.

Run make help to inspect all available development and build targets.

For additional information on operator development, refer to the Kubebuilder Documentation.

License
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file except in compliance with the License. You may obtain a copy of the License at

http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software distributed under the License is distributed on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the License for the specific language governing permissions and limitations under the License.