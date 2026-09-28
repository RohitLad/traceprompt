// Session + project state for the Traceprompt SPA.
// Token lives in localStorage so refreshes keep the session;
// nothing sensitive besides the JWT (short TTL, server-validated).

import { writable } from 'svelte/store';

export const API_BASE: string =
	import.meta.env.VITE_API_BASE_URL?.replace(/\/$/, '') ?? 'http://localhost:3000';

const TOKEN_KEY = 'traceprompt.token';
const PROJECT_KEY = 'traceprompt.projectId';

export const token = writable<string | null>(
	typeof localStorage !== 'undefined' ? localStorage.getItem(TOKEN_KEY) : null
);
export const currentProjectId = writable<string | null>(
	typeof localStorage !== 'undefined' ? localStorage.getItem(PROJECT_KEY) : null
);

export function setToken(t: string | null) {
	token.set(t);
	if (typeof localStorage === 'undefined') return;
	if (t) localStorage.setItem(TOKEN_KEY, t);
	else localStorage.removeItem(TOKEN_KEY);
}

export function setProject(id: string | null) {
	currentProjectId.set(id);
	if (typeof localStorage === 'undefined') return;
	if (id) localStorage.setItem(PROJECT_KEY, id);
	else localStorage.removeItem(PROJECT_KEY);
}

export function authHeaders(t: string | null): Record<string, string> {
	return t ? { Authorization: `Bearer ${t}` } : {};
}

export interface HealthResponse {
	status: string;
	service: string;
}

export async function fetchHealth(base: string = API_BASE): Promise<HealthResponse> {
	const res = await fetch(`${base}/api/health`);
	if (!res.ok) throw new Error(`health check failed: ${res.status}`);
	return (await res.json()) as HealthResponse;
}

export interface Project {
	id: string;
	organizationId: string;
	name: string;
}

export async function login(base: string, email: string, password: string) {
	const res = await fetch(`${base}/api/v1/auth/login`, {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify({ email, password })
	});
	if (!res.ok) throw new Error(parseError(await res.text(), res.status));
	return (await res.json()) as { token: string; user: unknown; org: { id: string; name: string } };
}

export async function register(
	base: string,
	email: string,
	password: string,
	name: string,
	orgName: string
) {
	const res = await fetch(`${base}/api/v1/auth/register`, {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify({ email, password, name, orgName })
	});
	if (!res.ok) throw new Error(parseError(await res.text(), res.status));
	return (await res.json()) as { token: string; user: unknown; org: { id: string; name: string } };
}

export async function listProjects(base: string, t: string): Promise<Project[]> {
	const res = await fetch(`${base}/api/v1/projects`, { headers: authHeaders(t) });
	if (!res.ok) throw new Error(parseError(await res.text(), res.status));
	const body = (await res.json()) as { data: Project[] };
	return body.data ?? [];
}

export interface UITrace {
	traceId: string;
	name: string;
	userId?: string | null;
	sessionId?: string | null;
	tags: string[];
	observationCount: number;
}

export interface UIObservation {
	observationId?: string;
	id: string;
	traceId: string;
	type: string;
	name: string;
	startTime: string;
	endTime?: string | null;
	input?: string | null;
	output?: string | null;
	model?: string | null;
	inputUsage?: number | null;
	outputUsage?: number | null;
	totalUsage?: number | null;
	level: string;
	statusMessage?: string | null;
}

export function tracesListUrl(base: string, projectId: string, limit = 25, cursor?: string): string {
	const q = new URLSearchParams({ limit: String(limit) });
	if (cursor) q.set('cursor', cursor);
	return `${base}/api/v1/projects/${projectId}/traces?${q}`;
}

export async function listTraces(
	base: string,
	t: string,
	projectId: string,
	limit = 25,
	cursor?: string
): Promise<{ data: UITrace[]; cursor: string | null }> {
	const res = await fetch(tracesListUrl(base, projectId, limit, cursor), { headers: authHeaders(t) });
	if (!res.ok) throw new Error(parseError(await res.text(), res.status));
	const body = (await res.json()) as { data: UITrace[]; meta: { cursor: string | null } };
	return { data: body.data ?? [], cursor: body.meta?.cursor ?? null };
}

export async function getTrace(
	base: string,
	t: string,
	projectId: string,
	traceId: string
): Promise<{ name: string; userId?: string | null; observations: UIObservation[] }> {
	const res = await fetch(`${base}/api/v1/projects/${projectId}/traces/${traceId}`, {
		headers: authHeaders(t)
	});
	if (!res.ok) throw new Error(parseError(await res.text(), res.status));
	const body = (await res.json()) as {
		data: { name: string; userId?: string | null; observations: UIObservation[] };
	};
	return body.data;
}

export function parseError(text: string, status: number): string {
	try {
		const body = JSON.parse(text) as { error?: string };
		if (body.error) return body.error;
	} catch {
		/* fall through */
	}
	return `request failed (${status})`;
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
