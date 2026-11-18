import ipaddress
import socket
from urllib.parse import urlparse
from fastapi import Header, HTTPException, status
from .config import API_TOKEN

BLOCKED_HOSTS = {
    "kubernetes.default.svc",
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
    hostname = parsed.hostname
    if not hostname:
        raise HTTPException(status_code=422, detail="Invalid URL: missing hostname.")
    if hostname.lower() in BLOCKED_HOSTS:
        raise HTTPException(status_code=422, detail=f"Security Violation: Host '{hostname}' is restricted.")

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
