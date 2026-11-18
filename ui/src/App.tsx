import React, { useEffect, useState } from 'react';
import {
  LayoutDashboard,
  Shield,
  PlusCircle,
  BarChart3,
  Search,
  Menu,
  RefreshCw,
  Trash2,
  Activity,
  CheckCircle2,
  Sliders,
  ArrowRight,
  Layers,
} from 'lucide-react';
import { api, type K8sService, type OverviewMetrics, type ShadowPolicy } from './api';

type Page = 'overview' | 'policies' | 'create' | 'analytics';

export default function App() {
  const [activePage, setActivePage] = useState<Page>('overview');
  const [sidebarOpen, setSidebarOpen] = useState(true);
  const [searchQuery, setSearchQuery] = useState('');
  const [statusFilter, setStatusFilter] = useState<'all' | 'active' | 'paused'>('all');
  const [timeWindow, setTimeWindow] = useState('15m');

  const [health, setHealth] = useState('CHECKING');
  const [policies, setPolicies] = useState<ShadowPolicy[]>([]);
  const [services, setServices] = useState<K8sService[]>([]);
  const [metrics, setMetrics] = useState<OverviewMetrics | null>(null);
  const [loading, setLoading] = useState(false);

  // Create Policy Form State (NyaProxy style)
  const [policyName, setPolicyName] = useState('');
  const [namespace, setNamespace] = useState('default');
  const [sourceService, setSourceService] = useState('order-service');
  const [targetService, setTargetService] = useState('shadow-service');
  const [mirrorPct, setMirrorPct] = useState(25);
  const [injectShadowHeader, setInjectShadowHeader] = useState(true);
  const [ignoredFieldsRaw, setIgnoredFieldsRaw] = useState('timestamp, traceId, requestId');
  const [formError, setFormError] = useState('');
  const [submitting, setSubmitting] = useState(false);

  const loadAll = async () => {
    setLoading(true);
    try {
      const [h, p, s, m] = await Promise.all([
        api.getHealth(),
        api.getPolicies(),
        api.getServices(),
        api.getOverviewMetrics(timeWindow),
      ]);
      setHealth(h.status);
      setPolicies(p || []);
      setServices(s || []);
      setMetrics(m);
    } catch {
      setHealth('DISCONNECTED');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    loadAll();
    const id = setInterval(loadAll, 10000);
    return () => clearInterval(id);
  }, [timeWindow]);

  const handleCreateSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setFormError('');
    setSubmitting(true);
    try {
      const ignoredFields = ignoredFieldsRaw
        .split(',')
        .map((f) => f.trim())
        .filter(Boolean);

      await api.createPolicy({
        name: policyName,
        namespace,
        sourceService,
        targetService,
        mirrorPercentage: mirrorPct,
        ignoredFields,
        headers: injectShadowHeader ? [{ name: 'x-shadow-test', exact: 'true' }] : [],
      });
      setPolicyName('');
      await loadAll();
      setActivePage('policies');
    } catch (err: any) {
      setFormError(err.message);
    } finally {
      setSubmitting(false);
    }
  };

  const handleToggleMirror = async (p: ShadowPolicy) => {
    const nextPct = p.spec.mirrorPercentage > 0 ? 0 : 50;
    await api.updatePolicy(p.metadata.namespace, p.metadata.name, nextPct);
    await loadAll();
  };

  const handleDelete = async (p: ShadowPolicy) => {
    await api.deletePolicy(p.metadata.namespace, p.metadata.name);
    await loadAll();
  };

  const filteredPolicies = policies.filter((p) => {
    const matchesSearch =
      p.metadata.name.toLowerCase().includes(searchQuery.toLowerCase()) ||
      p.spec.sourceService.toLowerCase().includes(searchQuery.toLowerCase()) ||
      p.spec.targetService.toLowerCase().includes(searchQuery.toLowerCase());

    if (statusFilter === 'active') return matchesSearch && p.spec.mirrorPercentage > 0;
    if (statusFilter === 'paused') return matchesSearch && p.spec.mirrorPercentage === 0;
    return matchesSearch;
  });

  const activeCount = policies.filter((p) => p.spec.mirrorPercentage > 0).length;

  return (
    <div className="min-h-screen flex bg-[#090D16] text-slate-100">
      {/* Collapsible Left Sidebar */}
      {sidebarOpen && (
        <aside className="w-60 bg-[#0D1320] border-r border-slate-800/80 flex flex-col justify-between shrink-0">
          <div>
            <div className="h-14 px-4 flex items-center gap-2.5 border-b border-slate-800/80">
              <div className="w-7 h-7 rounded-lg bg-indigo-600 flex items-center justify-center font-bold text-sm">
                SC
              </div>
              <span className="font-semibold tracking-tight text-white">ShadowCast</span>
            </div>

            <div className="p-3">
              <button
                onClick={() => setActivePage('create')}
                className="w-full py-2 px-3 rounded-lg bg-indigo-600/15 border border-indigo-500/40 text-indigo-300 hover:bg-indigo-600/25 text-sm font-medium flex items-center justify-center gap-2 transition"
              >
                <PlusCircle className="w-4 h-4" />
                Compose Policy
              </button>
            </div>

            <nav className="px-2 space-y-1">
              {[
                { id: 'overview', label: 'Overview Dashboard', icon: LayoutDashboard },
                { id: 'policies', label: 'Shadow Policies', icon: Shield },
                { id: 'create', label: 'Create Policy', icon: Sliders },
                { id: 'analytics', label: 'Traffic Analytics', icon: BarChart3 },
              ].map((item) => {
                const Icon = item.icon;
                const active = activePage === item.id;
                return (
                  <button
                    key={item.id}
                    onClick={() => setActivePage(item.id as Page)}
                    className={`w-full flex items-center gap-3 px-3 py-2.5 rounded-lg text-sm transition ${
                      active
                        ? 'bg-indigo-600 text-white font-medium'
                        : 'text-slate-400 hover:text-slate-100 hover:bg-slate-800/50'
                    }`}
                  >
                    <Icon className="w-4 h-4" />
                    {item.label}
                  </button>
                );
              })}
            </nav>
          </div>

          <div className="p-4 border-t border-slate-800/80 text-xs text-slate-500">
            <div>Control Plane: Envoy xDS</div>
            <div className="mt-1 text-slate-400">API v1alpha1</div>
          </div>
        </aside>
      )}

      {/* Main Content Area */}
      <div className="flex-1 flex flex-col min-w-0">
        {/* Top Bar with Menu Toggle, Search & Create Workspace */}
        <header className="h-14 bg-[#0D1320] border-b border-slate-800/80 px-4 flex items-center justify-between gap-4">
          <div className="flex items-center gap-3 flex-1 max-w-xl">
            <button
              onClick={() => setSidebarOpen(!sidebarOpen)}
              className="p-2 rounded-lg text-slate-400 hover:text-white hover:bg-slate-800"
              title="Toggle Sidebar"
            >
              <Menu className="w-4 h-4" />
            </button>

            <div className="relative flex-1">
              <Search className="w-4 h-4 text-slate-500 absolute left-3 top-2.5" />
              <input
                type="text"
                value={searchQuery}
                onChange={(e) => setSearchQuery(e.target.value)}
                placeholder="Search workspaces, policies, or services..."
                className="w-full bg-[#131B2E] border border-slate-800 rounded-lg pl-9 pr-3 py-1.5 text-sm text-slate-200 placeholder-slate-500 focus:outline-none focus:border-indigo-500"
              />
            </div>
          </div>

          <div className="flex items-center gap-3">
            <button
              onClick={() => setActivePage('create')}
              className="px-3 py-1.5 rounded-lg bg-indigo-600 hover:bg-indigo-500 text-white text-xs font-medium flex items-center gap-1.5 transition"
            >
              <PlusCircle className="w-3.5 h-3.5" />
              New Policy
            </button>

            <button
              onClick={loadAll}
              className="p-1.5 rounded-lg bg-[#131B2E] border border-slate-800 text-slate-400 hover:text-white"
              title="Refresh"
            >
              <RefreshCw className={`w-4 h-4 ${loading ? 'animate-spin' : ''}`} />
            </button>

            <div className="flex items-center gap-2 px-3 py-1 rounded-full bg-emerald-500/10 border border-emerald-500/30 text-xs text-emerald-400">
              <span className="w-2 h-2 rounded-full bg-emerald-400 animate-pulse" />
              {health}
            </div>
          </div>
        </header>

        {/* Dynamic Page Body */}
        <main className="flex-1 p-6 overflow-y-auto">
          {/* VIEW 1: OVERVIEW DASHBOARD (Open 360 Style) */}
          {activePage === 'overview' && (
            <div className="space-y-6">
              <div className="flex items-center justify-between">
                <div>
                  <h1 className="text-xl font-semibold text-white">Application Health & Shadow Overview</h1>
                  <p className="text-xs text-slate-400 mt-0.5">
                    Real-time Envoy traffic mirroring telemetry across Kubernetes services
                  </p>
                </div>

                <div className="flex items-center gap-2">
                  <select
                    value={timeWindow}
                    onChange={(e) => setTimeWindow(e.target.value)}
                    className="bg-[#131B2E] border border-slate-800 rounded-lg px-3 py-1.5 text-xs text-slate-200"
                  >
                    <option value="5m">Last 5 Mins</option>
                    <option value="15m">Last 15 Mins</option>
                    <option value="1h">Last 1 Hour</option>
                  </select>
                </div>
              </div>

              {/* 4 KPI Cards */}
              <div className="grid grid-cols-1 md:grid-cols-4 gap-4">
                <div className="bg-[#111827] border border-slate-800 rounded-xl p-4">
                  <div className="text-xs text-slate-400">Active Shadow Policies</div>
                  <div className="text-2xl font-bold text-white mt-2">
                    {activeCount} <span className="text-xs font-normal text-slate-500">/ {policies.length} total</span>
                  </div>
                </div>
                <div className="bg-[#111827] border border-slate-800 rounded-xl p-4">
                  <div className="text-xs text-slate-400">Primary Throughput</div>
                  <div className="text-2xl font-bold text-sky-400 mt-2">
                    {(metrics?.primary_rps ?? 0).toFixed(2)} <span className="text-xs font-normal">req/s</span>
                  </div>
                </div>
                <div className="bg-[#111827] border border-slate-800 rounded-xl p-4">
                  <div className="text-xs text-slate-400">Mirrored Shadow RPS</div>
                  <div className="text-2xl font-bold text-indigo-400 mt-2">
                    {(metrics?.shadow_rps ?? 0).toFixed(2)} <span className="text-xs font-normal">req/s</span>
                  </div>
                </div>
                <div className="bg-[#111827] border border-slate-800 rounded-xl p-4">
                  <div className="text-xs text-slate-400">Shadow 2xx Pass Rate</div>
                  <div className="text-2xl font-bold text-emerald-400 mt-2">
                    {(metrics?.pass_rate_pct ?? 100).toFixed(1)}%
                  </div>
                </div>
              </div>

              {/* Split Panels: Traffic Bars + Active Route Table */}
              <div className="grid grid-cols-1 lg:grid-cols-3 gap-4">
                <div className="lg:col-span-2 bg-[#111827] border border-slate-800 rounded-xl p-5">
                  <div className="flex items-center justify-between mb-4">
                    <h2 className="text-sm font-medium text-slate-200">Request Distribution (Primary vs Shadow)</h2>
                    <button
                      onClick={() => setActivePage('analytics')}
                      className="text-xs text-indigo-400 hover:text-indigo-300 flex items-center gap-1"
                    >
                      Deep Analytics <ArrowRight className="w-3.5 h-3.5" />
                    </button>
                  </div>

                  {/* Visual Bars */}
                  <div className="h-48 flex items-end gap-3 pt-6 px-2 border-b border-slate-800">
                    {[62, 74, 68, 85, 92, 78, 88, 95, 81, 90, 84, 96].map((h, i) => (
                      <div key={i} className="flex-1 flex flex-col items-center gap-1">
                        <div className="w-full flex flex-col justify-end h-36 gap-0.5">
                          <div
                            style={{ height: `${h * 0.45}%` }}
                            className="w-full bg-indigo-500/80 rounded-t-sm"
                            title="Shadow Mirrored"
                          />
                          <div
                            style={{ height: `${h * 0.55}%` }}
                            className="w-full bg-sky-500/80 rounded-b-sm"
                            title="Primary Live"
                          />
                        </div>
                        <span className="text-[10px] text-slate-500">{i + 1}m</span>
                      </div>
                    ))}
                  </div>
                  <div className="flex items-center gap-6 mt-3 text-xs text-slate-400">
                    <span className="flex items-center gap-2">
                      <span className="w-2.5 h-2.5 rounded-sm bg-sky-500" /> Primary Upstream
                    </span>
                    <span className="flex items-center gap-2">
                      <span className="w-2.5 h-2.5 rounded-sm bg-indigo-500" /> Shadow Upstream
                    </span>
                  </div>
                </div>

                {/* Right Service Summary Table */}
                <div className="bg-[#111827] border border-slate-800 rounded-xl p-5">
                  <div className="flex items-center justify-between mb-4">
                    <h2 className="text-sm font-medium text-slate-200">Active Policies</h2>
                    <button
                      onClick={() => setActivePage('policies')}
                      className="text-xs text-indigo-400 hover:underline"
                    >
                      Manage all
                    </button>
                  </div>

                  <div className="space-y-3">
                    {policies.map((p) => (
                      <div
                        key={p.metadata.name}
                        className="p-3 rounded-lg bg-[#0B101D] border border-slate-800/80 flex items-center justify-between"
                      >
                        <div>
                          <div className="text-sm font-medium text-white">{p.metadata.name}</div>
                          <div className="text-xs text-slate-400 mt-0.5">
                            {p.spec.sourceService} → {p.spec.targetService}
                          </div>
                        </div>
                        <span className="px-2.5 py-1 rounded-md bg-indigo-500/15 text-indigo-300 text-xs font-semibold">
                          {p.spec.mirrorPercentage}%
                        </span>
                      </div>
                    ))}
                  </div>
                </div>
              </div>
            </div>
          )}

          {/* VIEW 2: SHADOW POLICIES (Firefly Governance Style) */}
          {activePage === 'policies' && (
            <div className="space-y-4">
              <div>
                <div className="text-xs text-slate-500">Home / Shadow Policies</div>
                <h1 className="text-xl font-semibold text-white mt-1">Traffic Shadowing Policies</h1>
              </div>

              <div className="grid grid-cols-1 lg:grid-cols-4 gap-5">
                {/* Left Filter Sidebar */}
                <div className="bg-[#111827] border border-slate-800 rounded-xl p-4 space-y-2 h-fit">
                  <div className="text-xs font-semibold uppercase tracking-wider text-slate-500 mb-2">
                    Filter by State
                  </div>
                  {[
                    { id: 'all', label: `All Policies (${policies.length})` },
                    { id: 'active', label: `Active Mirroring (${activeCount})` },
                    { id: 'paused', label: `Paused / 0% (${policies.length - activeCount})` },
                  ].map((f) => (
                    <button
                      key={f.id}
                      onClick={() => setStatusFilter(f.id as any)}
                      className={`w-full text-left px-3 py-2 rounded-lg text-sm transition ${
                        statusFilter === f.id
                          ? 'bg-indigo-600/20 border border-indigo-500/40 text-indigo-300'
                          : 'text-slate-400 hover:bg-slate-800/50'
                      }`}
                    >
                      {f.label}
                    </button>
                  ))}
                </div>

                {/* Right Governance Table */}
                <div className="lg:col-span-3 bg-[#111827] border border-slate-800 rounded-xl overflow-hidden">
                  <table className="w-full text-left text-sm">
                    <thead className="bg-[#0D1320] border-b border-slate-800 text-xs text-slate-400">
                      <tr>
                        <th className="py-3 px-4">Policy Name</th>
                        <th className="py-3 px-4">Routing Path</th>
                        <th className="py-3 px-4">Mirror %</th>
                        <th className="py-3 px-4">Status</th>
                        <th className="py-3 px-4 text-right">Actions</th>
                      </tr>
                    </thead>
                    <tbody className="divide-y divide-slate-800/70">
                      {filteredPolicies.map((p) => (
                        <tr key={p.metadata.name} className="hover:bg-slate-800/30">
                          <td className="py-3.5 px-4 font-medium text-white">
                            {p.metadata.name}
                            <div className="text-xs text-slate-500">ns: {p.metadata.namespace}</div>
                          </td>
                          <td className="py-3.5 px-4 text-slate-300">
                            <span className="text-sky-400">{p.spec.sourceService}</span>
                            <span className="mx-2 text-slate-600">→</span>
                            <span className="text-indigo-400">{p.spec.targetService}</span>
                          </td>
                          <td className="py-3.5 px-4">
                            <span className="px-2.5 py-1 rounded bg-slate-800 text-white font-mono text-xs">
                              {p.spec.mirrorPercentage}%
                            </span>
                          </td>
                          <td className="py-3.5 px-4">
                            {p.spec.mirrorPercentage > 0 ? (
                              <span className="inline-flex items-center gap-1.5 text-xs text-emerald-400">
                                <CheckCircle2 className="w-3.5 h-3.5" /> Active
                              </span>
                            ) : (
                              <span className="text-xs text-amber-400">Paused (0%)</span>
                            )}
                          </td>
                          <td className="py-3.5 px-4 text-right space-x-2">
                            <button
                              onClick={() => handleToggleMirror(p)}
                              className="px-2.5 py-1 rounded bg-slate-800 hover:bg-slate-700 text-xs text-slate-200"
                            >
                              {p.spec.mirrorPercentage > 0 ? 'Pause' : 'Resume 50%'}
                            </button>
                            <button
                              onClick={() => handleDelete(p)}
                              className="p-1.5 rounded bg-rose-500/10 hover:bg-rose-500/20 text-rose-400 inline-flex align-middle"
                              title="Delete Policy"
                            >
                              <Trash2 className="w-3.5 h-3.5" />
                            </button>
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              </div>
            </div>
          )}

          {/* VIEW 3: CREATE POLICY EDITOR (NyaProxy Style) */}
          {activePage === 'create' && (
            <form onSubmit={handleCreateSubmit} className="max-w-3xl mx-auto space-y-5">
              <div>
                <div className="text-xs text-slate-500">Dashboard &gt; Shadow Policies &gt; Create</div>
                <h1 className="text-xl font-semibold text-white mt-1">Configure Shadow Policy</h1>
                <p className="text-xs text-slate-400">
                  Mirror live production requests asynchronously to a pre-staged Kubernetes service
                </p>
              </div>

              {formError && (
                <div className="p-3 rounded-lg bg-rose-500/15 border border-rose-500/40 text-xs text-rose-300">
                  {formError}
                </div>
              )}

              {/* Card 1: Policy Identity & Endpoints */}
              <div className="bg-[#111827] border border-slate-800 rounded-xl p-5 space-y-4">
                <div className="text-sm font-medium text-white border-b border-slate-800 pb-2.5">
                  Service Endpoints
                </div>

                <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                  <div>
                    <label className="block text-xs text-slate-400 mb-1.5">Policy Name</label>
                    <input
                      required
                      value={policyName}
                      onChange={(e) => setPolicyName(e.target.value)}
                      placeholder="e.g. checkout-v2-shadow"
                      className="w-full bg-[#0B101D] border border-slate-700 rounded-lg px-3 py-2 text-sm text-white"
                    />
                  </div>
                  <div>
                    <label className="block text-xs text-slate-400 mb-1.5">Namespace</label>
                    <input
                      value={namespace}
                      onChange={(e) => setNamespace(e.target.value)}
                      className="w-full bg-[#0B101D] border border-slate-700 rounded-lg px-3 py-2 text-sm text-white"
                    />
                  </div>
                </div>

                <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                  <div>
                    <label className="block text-xs text-slate-400 mb-1.5">
                      Primary Service (Live Version)
                    </label>
                    <select
                      value={sourceService}
                      onChange={(e) => setSourceService(e.target.value)}
                      className="w-full bg-[#0B101D] border border-slate-700 rounded-lg px-3 py-2 text-sm text-white"
                    >
                      {services
                        .filter((s) => s.namespace === namespace)
                        .map((s) => (
                          <option key={s.name} value={s.name}>
                            {s.name} ({s.clusterIP})
                          </option>
                        ))}
                    </select>
                  </div>

                  <div>
                    <label className="block text-xs text-slate-400 mb-1.5">
                      Shadow Service (Pre-Staged Candidate)
                    </label>
                    <select
                      value={targetService}
                      onChange={(e) => setTargetService(e.target.value)}
                      className="w-full bg-[#0B101D] border border-slate-700 rounded-lg px-3 py-2 text-sm text-white"
                    >
                      {services
                        .filter((s) => s.namespace === namespace)
                        .map((s) => (
                          <option key={s.name} value={s.name}>
                            {s.name} ({s.clusterIP})
                          </option>
                        ))}
                    </select>
                  </div>
                </div>
              </div>

              {/* Card 2: Mirror Percentage & Safety Headers */}
              <div className="bg-[#111827] border border-slate-800 rounded-xl p-5 space-y-4">
                <div className="flex items-center justify-between border-b border-slate-800 pb-2.5">
                  <span className="text-sm font-medium text-white">Traffic Sampling & Headers</span>
                  <span className="text-sm font-bold text-indigo-400">{mirrorPct}% Mirrored</span>
                </div>

                <input
                  type="range"
                  min={1}
                  max={100}
                  value={mirrorPct}
                  onChange={(e) => setMirrorPct(Number(e.target.value))}
                  className="w-full accent-indigo-500"
                />
                <div className="flex justify-between text-[11px] text-slate-500">
                  <span>1% (Canary)</span>
                  <span>50% (Half Traffic)</span>
                  <span>100% (Full Mirror)</span>
                </div>

                <div className="flex items-center justify-between pt-2">
                  <div>
                    <div className="text-sm text-slate-200">Match / Inject Shadow Header</div>
                    <div className="text-xs text-slate-500">Tags mirrored traffic with x-shadow-test: true</div>
                  </div>
                  <input
                    type="checkbox"
                    checked={injectShadowHeader}
                    onChange={(e) => setInjectShadowHeader(e.target.checked)}
                    className="w-4 h-4 accent-indigo-600"
                  />
                </div>

                <div>
                  <label className="block text-xs text-slate-400 mb-1.5">
                    Ignored Dynamic Fields (Comma-separated)
                  </label>
                  <input
                    value={ignoredFieldsRaw}
                    onChange={(e) => setIgnoredFieldsRaw(e.target.value)}
                    className="w-full bg-[#0B101D] border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-300"
                  />
                </div>
              </div>

              <div className="flex justify-end gap-3">
                <button
                  type="button"
                  onClick={() => setActivePage('policies')}
                  className="px-4 py-2 rounded-lg border border-slate-700 text-sm text-slate-300 hover:bg-slate-800"
                >
                  Cancel
                </button>
                <button
                  type="submit"
                  disabled={submitting}
                  className="px-5 py-2 rounded-lg bg-indigo-600 hover:bg-indigo-500 text-sm font-medium text-white"
                >
                  {submitting ? 'Applying to Kubernetes...' : 'Apply & Start Mirroring'}
                </button>
              </div>
            </form>
          )}

          {/* VIEW 4: TRAFFIC ANALYTICS & COMPARISON (Elastic / Grafana Style) */}
          {activePage === 'analytics' && (
            <div className="space-y-5">
              <div>
                <h1 className="text-xl font-semibold text-white">Traffic Differential & Latency Analytics</h1>
                <p className="text-xs text-slate-400">
                  HTTP 200 stability, P95 tail latency, and upstream throughput comparison
                </p>
              </div>

              <div className="grid grid-cols-1 lg:grid-cols-2 gap-4">
                <div className="bg-[#111827] border border-slate-800 rounded-xl p-5">
                  <div className="flex items-center gap-2 text-sm font-medium text-white mb-4">
                    <Activity className="w-4 h-4 text-emerald-400" />
                    Shadow HTTP Status Breakdown
                  </div>
                  <div className="space-y-3 text-sm">
                    <div className="flex justify-between">
                      <span className="text-slate-400">HTTP 2xx (Healthy)</span>
                      <span className="text-emerald-400 font-mono">
                        {(metrics?.shadow_2xx_rps ?? 0).toFixed(2)} req/s
                      </span>
                    </div>
                    <div className="flex justify-between">
                      <span className="text-slate-400">HTTP 5xx (Server Errors)</span>
                      <span className="text-rose-400 font-mono">
                        {(metrics?.shadow_5xx_rps ?? 0).toFixed(2)} req/s
                      </span>
                    </div>
                    <div className="flex justify-between">
                      <span className="text-slate-400">Shadow P95 Tail Latency</span>
                      <span className="text-sky-400 font-mono">
                        {(metrics?.shadow_p95_ms ?? 0).toFixed(1)} ms
                      </span>
                    </div>
                  </div>
                </div>

                <div className="bg-[#111827] border border-slate-800 rounded-xl p-5">
                  <div className="flex items-center gap-2 text-sm font-medium text-white mb-4">
                    <Layers className="w-4 h-4 text-indigo-400" />
                    Cluster Service Inventory ({services.length} services)
                  </div>
                  <div className="max-h-44 overflow-y-auto divide-y divide-slate-800 text-xs">
                    {services.map((s) => (
                      <div key={`${s.namespace}-${s.name}`} className="py-2 flex justify-between">
                        <span className="text-slate-200">{s.name}</span>
                        <span className="text-slate-500">
                          {s.namespace} • {s.clusterIP}
                        </span>
                      </div>
                    ))}
                  </div>
                </div>
              </div>
            </div>
          )}
        </main>
      </div>
    </div>
  );
}