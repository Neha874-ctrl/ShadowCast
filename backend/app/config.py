import os
from pathlib import Path

BASE_DIR = Path(__file__).resolve().parent.parent
DATA_DIR = Path(os.getenv("SHADOWCAST_DATA_DIR", str(BASE_DIR / "data")))
DATA_DIR.mkdir(parents=True, exist_ok=True)
STATE_FILE = DATA_DIR / "sessions.json"

# Security: API Token required by CLI and GTK Desktop App (no SSH needed)
API_TOKEN = os.getenv("SHADOWCAST_API_TOKEN", "sc-dev-token-change-in-prod")

# Cluster & Telemetry Endpoints
K8S_NAMESPACE = os.getenv("SHADOWCAST_NAMESPACE", "default")
PROMETHEUS_URL = os.getenv("PROMETHEUS_URL", "http://localhost:9090")
GRAFANA_URL = os.getenv("GRAFANA_URL", "http://localhost:3000")
GRAFANA_DASHBOARD_UID = os.getenv("GRAFANA_DASHBOARD_UID", "shadowcast-live")
ENVOY_METRICS_TARGET = os.getenv("ENVOY_METRICS_TARGET", "envoy-service.default.svc.cluster.local:9901")
DIFERENCIA_METRICS_TARGET = os.getenv("DIFERENCIA_METRICS_TARGET", "diferencia.default.svc.cluster.local:8060")
