import ipaddress
import socket
from urllib.parse import urlparse
from fastapi import Header, HTTPException, status
from .config import API_TOKEN
from .k8s_bridge import k8s_bridge

BLOCKED_HOSTS = {
    "kubernetes.default.svc",
    "kubernetes.default.svc.cluster.local",
    "kubernetes.default",
    "localhost",
    "metadata.google.internal",
}

def verify_bearer_token(authorization: str = Header(default="")) -> str:
    if not authorization.startswith("Bearer "):
        raise HTTPException(
            status_code=status.HTTP_401_UNAUTHORIZED,
            detail="Missing or malformed Authorization header. Expected: Bearer <token>",
        )
    token = authorization.removeprefix("Bearer ").strip()
    if token != API_TOKEN:
        raise HTTPException(
            status_code=status.HTTP_403_FORBIDDEN,
            detail="Invalid ShadowCast API token.",
        )
    return token

def validate_url_ssrf(raw_url: str) -> str:
    parsed = urlparse(raw_url)
    hostname = (parsed.hostname or "").lower()
    if not hostname:
        raise HTTPException(status_code=422, detail="Invalid URL: missing hostname.")
    if hostname in BLOCKED_HOSTS:
        raise HTTPException(status_code=422, detail=f"Security Violation: Host '{hostname}' is restricted.")

    # Allow verified Kubernetes cluster services when backend runs outside cluster DNS (e.g. EC2/local host)
    if hostname.endswith(".svc.cluster.local") or hostname.endswith(".svc"):
        svc_name = hostname.split(".")[0]
        known_services = {s["name"] for s in k8s_bridge.list_cluster_services()}
        if svc_name in known_services:
            return f"k8s-service:{svc_name}"
        raise HTTPException(status_code=422, detail=f"Kubernetes service '{svc_name}' not found in cluster.")

    port = parsed.port or (443 if parsed.scheme == "https" else 80)
    try:
        addr_info = socket.getaddrinfo(hostname, port)
    except socket.gaierror:
        raise HTTPException(status_code=422, detail=f"DNS resolution failed for host: {hostname}")

    resolved_ip = addr_info[0][4][0]
    ip_obj = ipaddress.ip_address(resolved_ip)

    if ip_obj.is_loopback or ip_obj.is_link_local or str(ip_obj) == "169.254.169.254":
        raise HTTPException(
            status_code=422,
            detail=f"Security Violation: Target IP {resolved_ip} belongs to restricted loopback/metadata range.",
        )
    return resolved_ip
