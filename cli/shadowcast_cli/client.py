import httpx
import typer
from rich.console import Console
from .config import load_config

console = Console()

class ShadowCastAPIClient:
    def __init__(self):
        cfg = load_config()
        self.base_url = cfg["server_url"].rstrip("/")
        self.token = cfg["api_token"]
        if not self.token:
            console.print("[bold red]Error:[/bold red] Not logged in. Run: [cyan]shadowcast login --server <URL> --token <TOKEN>[/cyan]")
            raise typer.Exit(code=1)

    @property
    def headers(self) -> dict:
        return {"Authorization": f"Bearer {self.token}"}

    def request(self, method: str, path: str, **kwargs) -> dict | list:
        url = f"{self.base_url}{path}"
        try:
            with httpx.Client(timeout=10.0) as hc:
                resp = hc.request(method, url, headers=self.headers, **kwargs)
        except httpx.RequestError as exc:
            console.print(f"[bold red]Connection Error:[/bold red] Could not reach {self.base_url} ({exc})")
            raise typer.Exit(code=1)

        if resp.status_code >= 400:
            try:
                detail = resp.json().get("detail", resp.text)
            except Exception:
                detail = resp.text
            console.print(f"[bold red]API Error ({resp.status_code}):[/bold red] {detail}")
            raise typer.Exit(code=1)

        return resp.json()
