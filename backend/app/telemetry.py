from urllib.parse import urlparse
import httpx
from .config import PROMETHEUS_URL, GRAFANA_URL, GRAFANA_DASHBOARD_UID

async def prom_instant_query(query: str) -> float:
    try:
        async with httpx.AsyncClient(timeout=3.0) as hc:
            resp = await hc.get(f"{PROMETHEUS_URL}/api/v1/query", params={"query": query})
            data = resp.json()
            results = data.get("data", {}).get("result", [])
            if not results:
                return 0.0
            val = float(results[0]["value"][1])
            return 0.0 if (val != val) else round(val, 4)  # guard NaN
    except Exception:
        return 0.0

async def prom_range_query(query: str, minutes: int = 15, step: str = "15s") -> list[dict]:
    import time
    end = int(time.time())
    start = end - (minutes * 60)
    try:
        async with httpx.AsyncClient(timeout=4.0) as hc:
            resp = await hc.get(
                f"{PROMETHEUS_URL}/api/v1/query_range",
                params={"query": query, "start": start, "end": end, "step": step},
            )
            data = resp.json()
            results = data.get("data", {}).get("result", [])
            if not results:
                return []
            return [{"ts": int(pt[0]), "value": round(float(pt[1]), 4)} for pt in results[0].get("values", [])]
    except Exception:
        return []

def build_grafana_url(session: dict) -> str:
    pri_host = urlparse(session["primary_url"]).netloc
    shad_host = urlparse(session["shadow_url"]).netloc
    return (
        f"{GRAFANA_URL}/d/{GRAFANA_DASHBOARD_UID}/shadowcast-live"
        f"?orgId=1&var-session_id={session['id']}&var-primary_host={pri_host}"
        f"&var-shadow_host={shad_host}&from=now-15m&to=now&refresh=5s"
    )

async def collect_session_metrics(session: dict, window_sec: int = 300) -> dict:
    w = f"{window_sec}s"
    pri_rps = await prom_instant_query(f'sum(rate(envoy_cluster_upstream_rq_total{{envoy_cluster_name!~".*shadow.*"}}[{w}]))')
    shad_rps = await prom_instant_query(f'sum(rate(envoy_cluster_upstream_rq_total{{envoy_cluster_name=~".*shadow.*"}}[{w}]))')

    pri_2xx = await prom_instant_query(f'sum(rate(envoy_cluster_upstream_rq_xx{{envoy_cluster_name!~".*shadow.*",envoy_response_code_class="2"}}[{w}]))')
    shad_2xx = await prom_instant_query(f'sum(rate(envoy_cluster_upstream_rq_xx{{envoy_cluster_name=~".*shadow.*",envoy_response_code_class="2"}}[{w}]))')
    shad_5xx = await prom_instant_query(f'sum(rate(envoy_cluster_upstream_rq_xx{{envoy_cluster_name=~".*shadow.*",envoy_response_code_class="5"}}[{w}]))')

    pri_pass = round((pri_2xx / pri_rps) * 100, 2) if pri_rps > 0 else 100.0
    shad_pass = round((shad_2xx / shad_rps) * 100, 2) if shad_rps > 0 else 100.0
    shad_err_rate = round((shad_5xx / shad_rps), 4) if shad_rps > 0 else 0.0

    pri_p50 = await prom_instant_query(f'histogram_quantile(0.50, sum(rate(envoy_cluster_upstream_rq_time_bucket{{envoy_cluster_name!~".*shadow.*"}}[{w}])) by (le))')
    pri_p95 = await prom_instant_query(f'histogram_quantile(0.95, sum(rate(envoy_cluster_upstream_rq_time_bucket{{envoy_cluster_name!~".*shadow.*"}}[{w}])) by (le))')
    pri_p99 = await prom_instant_query(f'histogram_quantile(0.99, sum(rate(envoy_cluster_upstream_rq_time_bucket{{envoy_cluster_name!~".*shadow.*"}}[{w}])) by (le))')

    shad_p50 = await prom_instant_query(f'histogram_quantile(0.50, sum(rate(envoy_cluster_upstream_rq_time_bucket{{envoy_cluster_name=~".*shadow.*"}}[{w}])) by (le))')
    shad_p95 = await prom_instant_query(f'histogram_quantile(0.95, sum(rate(envoy_cluster_upstream_rq_time_bucket{{envoy_cluster_name=~".*shadow.*"}}[{w}])) by (le))')
    shad_p99 = await prom_instant_query(f'histogram_quantile(0.99, sum(rate(envoy_cluster_upstream_rq_time_bucket{{envoy_cluster_name=~".*shadow.*"}}[{w}])) by (le))')

    mismatches = await prom_instant_query(f'sum(rate(diferencia_mismatches_total[{w}]))')

    # Server-side Apdex calculation (T=250ms, F=1000ms)
    apdex_penalty = 0.0 if shad_p95 <= 250 else (0.15 if shad_p95 <= 1000 else 0.45)
    shad_apdex = round(max(0.0, min(1.0, (shad_pass / 100.0) - apdex_penalty)), 2)
    pri_apdex = round(max(0.0, min(1.0, (pri_pass / 100.0) - (0.0 if pri_p95 <= 250 else 0.15))), 2)

    return {
        "session_id": session["id"],
        "window_seconds": window_sec,
        "primary": {
            "rps": round(pri_rps, 2),
            "success_rate_2xx": pri_pass,
            "latency_p50_ms": round(pri_p50, 2),
            "latency_p95_ms": round(pri_p95, 2),
            "latency_p99_ms": round(pri_p99, 2),
            "apdex_score": pri_apdex,
        },
        "shadow": {
            "rps": round(shad_rps, 2),
            "success_rate_2xx": shad_pass,
            "error_rate_5xx": shad_err_rate,
            "latency_p50_ms": round(shad_p50, 2),
            "latency_p95_ms": round(shad_p95, 2),
            "latency_p99_ms": round(shad_p99, 2),
            "apdex_score": shad_apdex,
            "diferencia_mismatch_rps": round(mismatches, 3),
        },
        "delta": {
            "p95_latency_ms": round(shad_p95 - pri_p95, 2),
            "success_rate_drop": round(pri_pass - shad_pass, 2),
            "circuit_breaker_status": "TRIPPED" if session.get("circuit_breaker_active") else "NORMAL",
        },
    }
