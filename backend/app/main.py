import json
import time
from datetime import datetime, timezone
from urllib.parse import urlparse
from typing import Dict, List, Optional

import httpx
from fastapi import FastAPI, Depends, HTTPException
from fastapi.middleware.cors import CORSMiddleware
from pydantic import BaseModel, HttpUrl, Field
from apscheduler.schedulers.asyncio import AsyncIOScheduler

from .config import STATE_FILE, ENVOY_METRICS_TARGET, DIFERENCIA_METRICS_TARGET
from .security import verify_bearer_token, validate_url_ssrf
from .k8s_bridge import k8s_bridge
from .telemetry import collect_session_metrics, prom_range_query, prom_instant_query, build_grafana_url

app = FastAPI(title="ShadowCast EC2 Control Plane", version="1.0.0-PROD")

app.add_middleware(
    CORSMiddleware,
    allow_origins=["*"],
    allow_methods=["*"],
    allow_headers=["*"],
)

# --- PERSISTENT SERVER-SIDE SESSION STORE ---
def load_state() -> tuple[Dict[str, dict], List[dict]]:
    if STATE_FILE.exists():
        try:
            raw = json.loads(STATE_FILE.read_text())
            return raw.get("sessions", {}), raw.get("anomalies", [])
        except Exception:
            pass
    return {}, []

def save_state():
    STATE_FILE.write_text(json.dumps({"sessions": SESSIONS, "anomalies": ANOMALIES[-500:]}, indent=2))

SESSIONS, ANOMALIES = load_state()

# --- REQUEST MODELS ---
class PreflightRequest(BaseModel):
    primary_url: HttpUrl
    shadow_url: HttpUrl

class SessionCreateRequest(BaseModel):
    workspace: str = "default"
    name: str
    primary_url: HttpUrl
    shadow_url: HttpUrl
    mirror_percentage: int = Field(default=25, ge=0, le=100)
    inject_shadow_header: bool = True
    strip_auth_headers: bool = True
    circuit_breaker_threshold: float = Field(default=0.20, ge=0.01, le=1.0)

class SessionUpdatePercentRequest(BaseModel):
    mirror_percentage: int = Field(ge=0, le=100)

# --- PUBLIC INFRASTRUCTURE ENDPOINTS ---
@app.get("/healthz")
async def healthz():
    return {
        "status": "ok",
        "k8s_connected": k8s_bridge.connected,
        "active_sessions": sum(1 for s in SESSIONS.values() if s["status"] == "ACTIVE"),
    }

@app.get("/api/v1/prometheus/targets")
async def prometheus_http_sd():
    """Auto-activates Prometheus scraping for all ACTIVE sessions via http_sd_configs."""
    targets = []
    for s in SESSIONS.values():
        if s["status"] == "ACTIVE":
            targets.append({
                "targets": [ENVOY_METRICS_TARGET, DIFERENCIA_METRICS_TARGET],
                "labels": {
                    "job": "shadowcast-active-sessions",
                    "session_id": s["id"],
                    "workspace": s.get("workspace", "default"),
                    "primary_host": urlparse(s["primary_url"]).netloc,
                    "shadow_host": urlparse(s["shadow_url"]).netloc,
                },
            })
    return targets

# --- AUTHENTICATED CLI & GTK DESKTOP ENDPOINTS ---
@app.get("/api/v1/auth/verify", dependencies=[Depends(verify_bearer_token)])
async def verify_client_auth():
    return {
        "authenticated": True,
        "k8s_connected": k8s_bridge.connected,
        "total_sessions": len(SESSIONS),
    }

@app.get("/api/v1/cluster/services", dependencies=[Depends(verify_bearer_token)])
async def get_cluster_services():
    return k8s_bridge.list_cluster_services()

@app.post("/api/v1/validation/preflight", dependencies=[Depends(verify_bearer_token)])
async def preflight_validation(req: PreflightRequest):
    pri_str, shad_str = str(req.primary_url), str(req.shadow_url)
    pri_ip = validate_url_ssrf(pri_str)
    shad_ip = validate_url_ssrf(shad_str)

    async def probe(url: str, ip: str) -> dict:
        t0 = time.perf_counter()
        if ip.startswith("k8s-service:"):
            svc_name = ip.split(":", 1)[1]
            known = {s["name"]: s for s in k8s_bridge.list_cluster_services()}
            if svc_name in known:
                return {
                    "reachable": True,
                    "status_code": 200,
                    "latency_ms": round((time.perf_counter() - t0) * 1000, 2),
                    "resolved_ip": f"cluster-svc ({svc_name})",
                }
        try:
            async with httpx.AsyncClient(timeout=4.0, verify=False) as hc:
                r = await hc.get(url)
                return {
                    "reachable": True,
                    "status_code": r.status_code,
                    "latency_ms": round((time.perf_counter() - t0) * 1000, 2),
                    "resolved_ip": ip,
                }
        except Exception as exc:
            return {
                "reachable": False,
                "status_code": 0,
                "latency_ms": round((time.perf_counter() - t0) * 1000, 2),
                "resolved_ip": ip,
                "error": str(exc),
            }

    pri_check = await probe(pri_str, pri_ip)
    shad_check = await probe(shad_str, shad_ip)
    return {
        "valid": pri_check["reachable"] and shad_check["reachable"],
        "primary_check": pri_check,
        "shadow_check": shad_check,
    }

@app.post("/api/v1/sessions", status_code=201, dependencies=[Depends(verify_bearer_token)])
async def create_session(req: SessionCreateRequest):
    validate_url_ssrf(str(req.primary_url))
    validate_url_ssrf(str(req.shadow_url))

    slug = "".join(c if c.isalnum() else "-" for c in req.name.lower()).strip("-")
    session_id = f"sc-{slug}-{int(time.time())}"
    session = {
        "id": session_id,
        "workspace": req.workspace,
        "name": req.name,
        "primary_url": str(req.primary_url),
        "shadow_url": str(req.shadow_url),
        "mirror_percentage": req.mirror_percentage,
        "inject_shadow_header": req.inject_shadow_header,
        "strip_auth_headers": req.strip_auth_headers,
        "circuit_breaker_threshold": req.circuit_breaker_threshold,
        "circuit_breaker_active": False,
        "status": "INITIALIZED",
        "prometheus_active": False,
        "grafana_url": None,
        "created_at": datetime.now(timezone.utc).isoformat(),
    }
    SESSIONS[session_id] = session
    save_state()
    return session

@app.get("/api/v1/sessions", dependencies=[Depends(verify_bearer_token)])
async def list_sessions():
    return list(SESSIONS.values())

@app.get("/api/v1/sessions/{session_id}", dependencies=[Depends(verify_bearer_token)])
async def get_session(session_id: str):
    if session_id not in SESSIONS:
        raise HTTPException(status_code=404, detail="Session not found")
    return SESSIONS[session_id]

@app.post("/api/v1/sessions/{session_id}/apply", dependencies=[Depends(verify_bearer_token)])
async def apply_session(session_id: str):
    session = SESSIONS.get(session_id)
    if not session:
        raise HTTPException(status_code=404, detail="Session not found")

    k8s_result = k8s_bridge.upsert_shadow_policy(session, session["mirror_percentage"])
    session["status"] = "ACTIVE"
    session["circuit_breaker_active"] = False
    session["prometheus_active"] = True
    session["grafana_url"] = build_grafana_url(session)
    session["k8s_sync"] = k8s_result
    session["updated_at"] = datetime.now(timezone.utc).isoformat()
    save_state()
    return session

@app.post("/api/v1/sessions/{session_id}/pause", dependencies=[Depends(verify_bearer_token)])
async def pause_session(session_id: str):
    session = SESSIONS.get(session_id)
    if not session:
        raise HTTPException(status_code=404, detail="Session not found")

    k8s_bridge.upsert_shadow_policy(session, 0)
    session["status"] = "PAUSED"
    session["prometheus_active"] = False
    session["updated_at"] = datetime.now(timezone.utc).isoformat()
    save_state()
    return session

@app.patch("/api/v1/sessions/{session_id}/percentage", dependencies=[Depends(verify_bearer_token)])
async def update_mirror_percentage(session_id: str, req: SessionUpdatePercentRequest):
    session = SESSIONS.get(session_id)
    if not session:
        raise HTTPException(status_code=404, detail="Session not found")

    session["mirror_percentage"] = req.mirror_percentage
    if session["status"] == "ACTIVE":
        k8s_bridge.upsert_shadow_policy(session, req.mirror_percentage)
    session["updated_at"] = datetime.now(timezone.utc).isoformat()
    save_state()
    return session

@app.delete("/api/v1/sessions/{session_id}", dependencies=[Depends(verify_bearer_token)])
async def delete_session(session_id: str):
    if session_id not in SESSIONS:
        raise HTTPException(status_code=404, detail="Session not found")
    k8s_bridge.delete_shadow_policy(session_id)
    del SESSIONS[session_id]
    save_state()
    return {"deleted": True, "session_id": session_id}

@app.get("/api/v1/sessions/{session_id}/metrics", dependencies=[Depends(verify_bearer_token)])
async def get_session_metrics(session_id: str, window: int = 300):
    session = SESSIONS.get(session_id)
    if not session:
        raise HTTPException(status_code=404, detail="Session not found")
    return await collect_session_metrics(session, window_sec=window)

@app.get("/api/v1/sessions/{session_id}/timeseries", dependencies=[Depends(verify_bearer_token)])
async def get_session_timeseries(session_id: str, minutes: int = 15):
    if session_id not in SESSIONS:
        raise HTTPException(status_code=404, detail="Session not found")
    pri_series = await prom_range_query('sum(rate(envoy_cluster_upstream_rq_total{envoy_cluster_name!~".*shadow.*"}[1m]))', minutes)
    shad_series = await prom_range_query('sum(rate(envoy_cluster_upstream_rq_total{envoy_cluster_name=~".*shadow.*"}[1m]))', minutes)
    return {"session_id": session_id, "primary_rps": pri_series, "shadow_rps": shad_series}

@app.get("/api/v1/anomalies", dependencies=[Depends(verify_bearer_token)])
async def list_anomalies(session_id: Optional[str] = None):
    if session_id:
        return [a for a in ANOMALIES if a.get("session_id") == session_id]
    return ANOMALIES[-100:]

# --- 5-SECOND BACKGROUND CIRCUIT BREAKER WATCHDOG ---
scheduler = AsyncIOScheduler()

async def circuit_breaker_watchdog():
    active = [s for s in SESSIONS.values() if s["status"] == "ACTIVE"]
    if not active:
        return

    err_query = (
        'sum(rate(envoy_cluster_upstream_rq_xx{envoy_cluster_name=~".*shadow.*",envoy_response_code_class="5"}[10s]))'
        ' / clamp_min(sum(rate(envoy_cluster_upstream_rq_total{envoy_cluster_name=~".*shadow.*"}[10s])), 0.001)'
    )
    err_rate = await prom_instant_query(err_query)

    tripped_any = False
    for s in active:
        if err_rate > s.get("circuit_breaker_threshold", 0.20):
            k8s_bridge.upsert_shadow_policy(s, 0)
            s["status"] = "CIRCUIT_BREAKER_TRIPPED"
            s["circuit_breaker_active"] = True
            s["prometheus_active"] = False
            ANOMALIES.append({
                "timestamp": datetime.now(timezone.utc).isoformat(),
                "session_id": s["id"],
                "type": "CIRCUIT_BREAKER_TRIP",
                "shadow_5xx_rate": round(err_rate * 100, 2),
                "details": f"Shadow 5xx rate ({round(err_rate*100,1)}%) exceeded threshold. Mirroring auto-zeroed.",
            })
            tripped_any = True

    if tripped_any:
        save_state()

@app.on_event("startup")
async def startup_event():
    scheduler.add_job(circuit_breaker_watchdog, "interval", seconds=5)
    scheduler.start()
