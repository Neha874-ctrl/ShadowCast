import time
import webbrowser
from typing import Optional
import typer
from rich.console import Console
from rich.table import Table
from rich.panel import Panel
from rich.live import Live

from .config import load_config, save_config
from .client import ShadowCastAPIClient

app = typer.Typer(help="ShadowCast CLI — Zero-Risk Traffic Shadowing & Differential Telemetry")
session_app = typer.Typer(help="Create, apply, pause, inspect, and delete shadow sessions")
app.add_typer(session_app, name="session")
console = Console()

@app.command()
def login(
    server: str = typer.Option(..., "--server", "-s", help="ShadowCast EC2 backend URL (e.g. https://shadowcast.example.com)"),
    token: str = typer.Option(..., "--token", "-t", prompt=True, hide_input=True, help="Bearer API token"),
    workspace: str = typer.Option("default", "--workspace", "-w", help="Default workspace name"),
):
    """Configure EC2 server endpoint and API token (shared with GTK Desktop App)."""
    cfg_path = save_config(server, token, workspace)
    client = ShadowCastAPIClient()
    info = client.request("GET", "/api/v1/auth/verify")
    console.print(Panel.fit(
        f"[bold green]Authenticated successfully![/bold green]\n"
        f"Server: [cyan]{server}[/cyan]\n"
        f"Kubernetes Connected: [bold]{info.get('k8s_connected')}[/bold]\n"
        f"Config Saved: [dim]{cfg_path}[/dim]",
        title="ShadowCast Login",
    ))

@app.command()
def status():
    """Check EC2 control plane and Kubernetes cluster connection status."""
    cfg = load_config()
    client = ShadowCastAPIClient()
    info = client.request("GET", "/api/v1/auth/verify")
    services = client.request("GET", "/api/v1/cluster/services")
    console.print(f"[bold]Server:[/bold] {cfg['server_url']}  |  [bold]K8s Connected:[/bold] {info['k8s_connected']}  |  [bold]Sessions:[/bold] {info['total_sessions']}")
    if services:
        tbl = Table(title="Discovered Kubernetes Services")
        tbl.add_column("Service", style="cyan")
        tbl.add_column("Namespace")
        tbl.add_column("Ports")
        tbl.add_column("Internal URL", style="green")
        for s in services:
            tbl.add_row(s["name"], s["namespace"], ",".join(map(str, s["ports"])), s["internal_url"])
        console.print(tbl)

@app.command()
def preflight(
    primary: str = typer.Option(..., "--primary", "-p", help="Primary deployed service URL"),
    shadow: str = typer.Option(..., "--shadow", "-s", help="Pre-staged candidate shadow URL"),
):
    """Run server-side SSRF security check and HTTP reachability probes."""
    client = ShadowCastAPIClient()
    res = client.request("POST", "/api/v1/validation/preflight", json={"primary_url": primary, "shadow_url": shadow})

    tbl = Table(title="Pre-Flight Validation Results")
    tbl.add_column("Target", style="bold")
    tbl.add_column("Reachable")
    tbl.add_column("HTTP Status")
    tbl.add_column("Latency")
    tbl.add_column("Resolved IP", style="cyan")

    for label, key in [("Primary (Baseline)", "primary_check"), ("Shadow (Candidate)", "shadow_check")]:
        chk = res[key]
        reach = "[green]YES[/green]" if chk["reachable"] else f"[red]NO ({chk.get('error', '')})[/red]"
        tbl.add_row(label, reach, str(chk["status_code"]), f"{chk['latency_ms']} ms", chk["resolved_ip"])
    console.print(tbl)

@session_app.command("create")
def session_create(
    name: str = typer.Option(..., "--name", "-n", help="Session name (e.g. checkout-v2)"),
    primary: str = typer.Option(..., "--primary", "-p", help="Primary deployed URL"),
    shadow: str = typer.Option(..., "--shadow", "-s", help="Pre-staged shadow URL"),
    percent: int = typer.Option(25, "--percent", "-m", min=0, max=100, help="Mirror percentage (0-100)"),
    workspace: Optional[str] = typer.Option(None, "--workspace", "-w", help="Workspace name"),
    apply_now: bool = typer.Option(False, "--apply", help="Immediately apply CRD and activate Prometheus/Grafana"),
):
    """Create a new traffic-shadowing session."""
    cfg = load_config()
    client = ShadowCastAPIClient()
    payload = {
        "workspace": workspace or cfg["default_workspace"],
        "name": name,
        "primary_url": primary,
        "shadow_url": shadow,
        "mirror_percentage": percent,
    }
    sess = client.request("POST", "/api/v1/sessions", json=payload)
    if apply_now:
        sess = client.request("POST", f"/api/v1/sessions/{sess['id']}/apply")

    console.print(Panel.fit(
        f"[bold green]Session Created:[/bold green] [cyan]{sess['id']}[/cyan]\n"
        f"Status: [bold]{sess['status']}[/bold] | Mirror: [bold]{sess['mirror_percentage']}%[/bold]\n"
        f"Primary: {sess['primary_url']}\n"
        f"Shadow:  {sess['shadow_url']}\n"
        f"Grafana: [underline]{sess.get('grafana_url') or 'Apply session to activate'}[/underline]",
        title="ShadowCast Session",
    ))

@session_app.command("list")
def session_list():
    """List all shadow sessions on the EC2 server."""
    client = ShadowCastAPIClient()
    sessions = client.request("GET", "/api/v1/sessions")
    tbl = Table(title="ShadowCast Sessions")
    tbl.add_column("Session ID", style="cyan")
    tbl.add_column("Workspace")
    tbl.add_column("Status")
    tbl.add_column("Mirror %", justify="right")
    tbl.add_column("Primary URL")
    tbl.add_column("Shadow URL")

    for s in sessions:
        st = s["status"]
        st_fmt = f"[green]{st}[/green]" if st == "ACTIVE" else (f"[red]{st}[/red]" if "TRIPPED" in st else f"[yellow]{st}[/yellow]")
        tbl.add_row(s["id"], s.get("workspace", "default"), st_fmt, f"{s['mirror_percentage']}%", s["primary_url"], s["shadow_url"])
    console.print(tbl)

@session_app.command("apply")
def session_apply(session_id: str = typer.Argument(..., help="Session ID to activate")):
    """Activate shadowing in Kubernetes/Envoy and enable Prometheus + Grafana telemetry."""
    client = ShadowCastAPIClient()
    sess = client.request("POST", f"/api/v1/sessions/{session_id}/apply")
    console.print(f"[bold green]ACTIVE:[/bold green] {sess['id']} ({sess['mirror_percentage']}% mirrored)")
    if sess.get("grafana_url"):
        console.print(f"[bold]Grafana Dashboard:[/bold] [cyan]{sess['grafana_url']}[/cyan]")

@session_app.command("pause")
def session_pause(session_id: str = typer.Argument(..., help="Session ID to pause (sets mirror to 0%)")):
    """Pause traffic shadowing (sets mirrorPercentage to 0)."""
    client = ShadowCastAPIClient()
    sess = client.request("POST", f"/api/v1/sessions/{session_id}/pause")
    console.print(f"[bold yellow]PAUSED:[/bold yellow] {sess['id']} (mirroring set to 0%)")

@session_app.command("percent")
def session_percent(
    session_id: str = typer.Argument(..., help="Session ID"),
    percent: int = typer.Argument(..., min=0, max=100, help="New mirror percentage (0-100)"),
):
    """Dynamically adjust mirror percentage (0-100%)."""
    client = ShadowCastAPIClient()
    sess = client.request("PATCH", f"/api/v1/sessions/{session_id}/percentage", json={"mirror_percentage": percent})
    console.print(f"[bold green]Updated:[/bold green] {sess['id']} -> [cyan]{sess['mirror_percentage']}%[/cyan]")

@session_app.command("delete")
def session_delete(session_id: str = typer.Argument(..., help="Session ID to delete")):
    """Delete session and remove its Kubernetes ShadowPolicy CRD."""
    client = ShadowCastAPIClient()
    client.request("DELETE", f"/api/v1/sessions/{session_id}")
    console.print(f"[bold red]Deleted:[/bold red] {session_id}")

def _render_metrics_table(m: dict) -> Table:
    pri, shad, delta = m["primary"], m["shadow"], m["delta"]
    tbl = Table(title=f"Live Differential Telemetry — {m['session_id']} (Window: {m['window_seconds']}s)")
    tbl.add_column("Metric", style="bold")
    tbl.add_column("Primary (Baseline)", justify="right", style="cyan")
    tbl.add_column("Shadow (Candidate)", justify="right", style="magenta")
    tbl.add_column("Delta / Health", justify="right")

    tbl.add_row("Throughput (RPS)", f"{pri['rps']}", f"{shad['rps']}", "-")
    tbl.add_row("2xx Pass Rate", f"{pri['success_rate_2xx']}%", f"{shad['success_rate_2xx']}%", f"-{delta['success_rate_drop']}%")
    tbl.add_row("P50 Latency", f"{pri['latency_p50_ms']} ms", f"{shad['latency_p50_ms']} ms", "-")
    tbl.add_row("P95 Latency", f"{pri['latency_p95_ms']} ms", f"{shad['latency_p95_ms']} ms", f"{delta['p95_latency_ms']:+} ms")
    tbl.add_row("P99 Latency", f"{pri['latency_p99_ms']} ms", f"{shad['latency_p99_ms']} ms", "-")
    tbl.add_row("Apdex Score", f"{pri['apdex_score']}", f"{shad['apdex_score']}", "PASS" if shad["apdex_score"] >= 0.85 else "WARN")
    tbl.add_row("Diferencia Mismatch/s", "-", f"{shad['diferencia_mismatch_rps']}", "-")
    cb = delta["circuit_breaker_status"]
    tbl.add_row("Circuit Breaker", "-", "-", f"[green]{cb}[/green]" if cb == "NORMAL" else f"[bold red]{cb}[/bold red]")
    return tbl

@app.command()
def metrics(
    session_id: str = typer.Argument(..., help="Session ID"),
    watch: bool = typer.Option(False, "--watch", "-w", help="Stream live metrics every 2 seconds"),
    window: int = typer.Option(300, "--window", help="Aggregation window in seconds"),
):
    """Fetch server-computed differential metrics (or stream live with --watch)."""
    client = ShadowCastAPIClient()
    if not watch:
        data = client.request("GET", f"/api/v1/sessions/{session_id}/metrics", params={"window": window})
        console.print(_render_metrics_table(data))
        return

    with Live(console=console, refresh_per_second=1) as live:
        while True:
            data = client.request("GET", f"/api/v1/sessions/{session_id}/metrics", params={"window": window})
            live.update(_render_metrics_table(data))
            time.sleep(2)

@app.command()
def grafana(
    session_id: str = typer.Argument(..., help="Session ID"),
    open_browser: bool = typer.Option(True, "--open/--no-open", help="Open Grafana URL in default browser"),
):
    """Get or launch the pre-filtered Grafana dashboard for a session."""
    client = ShadowCastAPIClient()
    sess = client.request("GET", f"/api/v1/sessions/{session_id}")
    url = sess.get("grafana_url")
    if not url:
        console.print("[yellow]Session is not active yet. Run:[/yellow] shadowcast session apply " + session_id)
        raise typer.Exit(code=1)
    console.print(f"[bold green]Grafana Dashboard URL:[/bold green] {url}")
    if open_browser:
        webbrowser.open(url)

if __name__ == "__main__":
    app()
