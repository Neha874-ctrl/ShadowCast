from urllib.parse import urlparse
from kubernetes import client, config
from .config import K8S_NAMESPACE

class K8sShadowBridge:
    def __init__(self):
        self.connected = False
        try:
            config.load_incluster_config()
            self.connected = True
        except Exception:
            try:
                config.load_kube_config()
                self.connected = True
            except Exception:
                self.connected = False

        self.custom_api = client.CustomObjectsApi() if self.connected else None
        self.core_api = client.CoreV1Api() if self.connected else None

    def list_cluster_services(self) -> list[dict]:
        if not self.core_api:
            return []
        svcs = self.core_api.list_namespaced_service(namespace=K8S_NAMESPACE)
        result = []
        for s in svcs.items:
            ports = [p.port for p in (s.spec.ports or [])]
            result.append({
                "name": s.metadata.name,
                "namespace": s.metadata.namespace,
                "ports": ports,
                "internal_url": f"http://{s.metadata.name}.{s.metadata.namespace}.svc.cluster.local:{ports[0]}" if ports else "",
            })
        return result

    def upsert_shadow_policy(self, session: dict, mirror_percentage: int) -> dict:
        if not self.custom_api:
            return {"synced": False, "reason": "Kubernetes client not configured"}

        pri_host = urlparse(session["primary_url"]).hostname or "order-service"
        shad_host = urlparse(session["shadow_url"]).hostname or "shadow-service"
        src_svc = pri_host.split(".")[0]
        tgt_svc = shad_host.split(".")[0]
        crd_name = session["id"].replace("_", "-")

        headers_spec = []
        if session.get("inject_shadow_header", True):
            headers_spec.append({"name": "X-Shadow-Permitted", "exact": "true"})

        body = {
            "apiVersion": "traffic.shadowcast.io/v1alpha1",
            "kind": "ShadowPolicy",
            "metadata": {
                "name": crd_name,
                "namespace": K8S_NAMESPACE,
                "annotations": {
                    "shadowcast.io/session-id": session["id"],
                    "shadowcast.io/workspace": session.get("workspace", "default"),
                    "shadowcast.io/primary-url": session["primary_url"],
                    "shadowcast.io/shadow-url": session["shadow_url"],
                    "shadowcast.io/strip-auth": str(session.get("strip_auth_headers", True)).lower(),
                },
            },
            "spec": {
                "sourceService": src_svc,
                "targetService": tgt_svc,
                "mirrorPercentage": int(mirror_percentage),
                "ignoredFields": session.get("ignored_fields", ["timestamp", "traceId", "requestId"]),
                "headers": headers_spec,
            },
        }

        try:
            self.custom_api.patch_namespaced_custom_object(
                group="traffic.shadowcast.io",
                version="v1alpha1",
                namespace=K8S_NAMESPACE,
                plural="shadowpolicies",
                name=crd_name,
                body=body,
            )
        except client.exceptions.ApiException as exc:
            if exc.status == 404:
                self.custom_api.create_namespaced_custom_object(
                    group="traffic.shadowcast.io",
                    version="v1alpha1",
                    namespace=K8S_NAMESPACE,
                    plural="shadowpolicies",
                    body=body,
                )
            else:
                raise
        return {"synced": True, "crd_name": crd_name, "sourceService": src_svc, "targetService": tgt_svc}

    def delete_shadow_policy(self, session_id: str) -> None:
        if not self.custom_api:
            return
        crd_name = session_id.replace("_", "-")
        try:
            self.custom_api.delete_namespaced_custom_object(
                group="traffic.shadowcast.io",
                version="v1alpha1",
                namespace=K8S_NAMESPACE,
                plural="shadowpolicies",
                name=crd_name,
            )
        except client.exceptions.ApiException as exc:
            if exc.status != 404:
                raise

k8s_bridge = K8sShadowBridge()
