const API_BASE = 'http://localhost:8090/api/v1';

export interface K8sService {
  name: string;
  namespace: string;
  clusterIP: string;
}

export interface HeaderMatch {
  name: string;
  value?: string;
  exact?: string;
}

export interface ShadowPolicy {
  metadata: {
    name: string;
    namespace: string;
    creationTimestamp: string;
  };
  spec: {
    sourceService: string;
    targetService: string;
    mirrorPercentage: number;
    ignoredFields?: string[];
    headers?: HeaderMatch[];
  };
  status?: {
    active: boolean;
    totalRequestsMirrored?: number;
  };
}

export interface OverviewMetrics {
  window: string;
  primary_rps: number;
  shadow_rps: number;
  shadow_2xx_rps: number;
  shadow_5xx_rps: number;
  pass_rate_pct: number;
  shadow_p95_ms: number;
}

export const api = {
  getHealth: async (): Promise<{ status: string }> => {
    const res = await fetch(`${API_BASE}/health`);
    return res.json();
  },
  getServices: async (): Promise<K8sService[]> => {
    const res = await fetch(`${API_BASE}/services`);
    return res.json();
  },
  getPolicies: async (): Promise<ShadowPolicy[]> => {
    const res = await fetch(`${API_BASE}/policies`);
    return res.json();
  },
  createPolicy: async (payload: {
    name: string;
    namespace: string;
    sourceService: string;
    targetService: string;
    mirrorPercentage: number;
    ignoredFields?: string[];
    headers?: HeaderMatch[];
  }) => {
    const res = await fetch(`${API_BASE}/policies`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload),
    });
    if (!res.ok) throw new Error((await res.json()).error || 'Failed to create policy');
    return res.json();
  },
  updatePolicy: async (namespace: string, name: string, mirrorPercentage: number) => {
    const res = await fetch(`${API_BASE}/policies/${namespace}/${name}`, {
      method: 'PATCH',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ mirrorPercentage }),
    });
    if (!res.ok) throw new Error('Failed to update policy');
    return res.json();
  },
  deletePolicy: async (namespace: string, name: string) => {
    const res = await fetch(`${API_BASE}/policies/${namespace}/${name}`, {
      method: 'DELETE',
    });
    if (!res.ok) throw new Error('Failed to delete policy');
    return res.json();
  },
  getOverviewMetrics: async (window = '15m'): Promise<OverviewMetrics> => {
    const res = await fetch(`${API_BASE}/metrics/overview?window=${window}`);
    return res.json();
  },
};
