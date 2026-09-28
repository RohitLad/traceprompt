// Typed API client for the Traceprompt backend.
// Keeps Langfuse-compatible naming (camelCase) so SDK semantics carry over.

export const API_BASE: string =
	import.meta.env.VITE_API_BASE_URL?.replace(/\/$/, '') ?? 'http://localhost:3000';

export interface HealthResponse {
	status: string;
	service: string;
}

export interface Trace {
	traceId: string;
	name: string;
	userId?: string | null;
	sessionId?: string | null;
	tags: string[];
}

export async function fetchHealth(base: string = API_BASE): Promise<HealthResponse> {
	const res = await fetch(`${base}/api/health`);
	if (!res.ok) throw new Error(`health check failed: ${res.status}`);
	return (await res.json()) as HealthResponse;
}

export function tracesUrl(params: { limit?: number; cursor?: string } = {}): string {
	const q = new URLSearchParams();
	if (params.limit) q.set('limit', String(params.limit));
	if (params.cursor) q.set('cursor', params.cursor);
	const suffix = q.size > 0 ? `?${q}` : '';
	return `${API_BASE}/api/public/v2/observations${suffix}`;
}

export function formatLatency(ms: number | null | undefined): string {
	if (ms == null) return '—';
	if (ms < 1000) return `${Math.round(ms)}ms`;
	return `${(ms / 1000).toFixed(2)}s`;
}
