// Session + project state for the Traceprompt SPA.
// Token lives in localStorage so refreshes keep the session;
// nothing sensitive besides the JWT (short TTL, server-validated).

import { writable } from 'svelte/store';

export const API_BASE: string =
	import.meta.env.VITE_API_BASE_URL?.replace(/\/$/, '') ?? 'http://localhost:3000';

const TOKEN_KEY = 'traceprompt.token';
const PROJECT_KEY = 'traceprompt.projectId';
const ORG_KEY = 'traceprompt.orgId';

export const token = writable<string | null>(
	typeof localStorage !== 'undefined' ? localStorage.getItem(TOKEN_KEY) : null
);
export const currentProjectId = writable<string | null>(
	typeof localStorage !== 'undefined' ? localStorage.getItem(PROJECT_KEY) : null
);
export const currentOrgId = writable<string | null>(
	typeof localStorage !== 'undefined' ? localStorage.getItem(ORG_KEY) : null
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

export function setOrg(id: string | null) {
	currentOrgId.set(id);
	if (typeof localStorage === 'undefined') return;
	if (id) localStorage.setItem(ORG_KEY, id);
	else localStorage.removeItem(ORG_KEY);
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

export async function createProject(
	base: string,
	t: string,
	name: string,
	organizationId?: string
): Promise<Project> {
	const res = await fetch(`${base}/api/v1/projects`, {
		method: 'POST',
		headers: { 'Content-Type': 'application/json', ...authHeaders(t) },
		// organizationId is optional server-side: omitted for single-org accounts.
		body: JSON.stringify(
			organizationId ? { name, organizationId } : { name }
		)
	});
	if (!res.ok) throw new Error(parseError(await res.text(), res.status));
	return (await res.json()) as Project;
}

export interface APIKey {
	id: string;
	name: string;
	publicKey: string;
	createdAt: string;
	revokedAt: string | null;
}

export async function listKeys(base: string, t: string, projectId: string): Promise<APIKey[]> {
	const res = await fetch(`${base}/api/v1/projects/${projectId}/keys`, { headers: authHeaders(t) });
	if (!res.ok) throw new Error(parseError(await res.text(), res.status));
	return (((await res.json()) as { data: APIKey[] }).data ?? []);
}

export async function createKey(
	base: string,
	t: string,
	projectId: string,
	name: string
): Promise<APIKey & { secret: string }> {
	const res = await fetch(`${base}/api/v1/projects/${projectId}/keys`, {
		method: 'POST',
		headers: { 'Content-Type': 'application/json', ...authHeaders(t) },
		body: JSON.stringify({ name })
	});
	if (!res.ok) throw new Error(parseError(await res.text(), res.status));
	return (await res.json()) as APIKey & { secret: string };
}

export async function revokeKey(base: string, t: string, projectId: string, keyId: string): Promise<void> {
	const res = await fetch(`${base}/api/v1/projects/${projectId}/keys/${keyId}/revoke`, {
		method: 'POST',
		headers: authHeaders(t)
	});
	if (!res.ok) throw new Error(parseError(await res.text(), res.status));
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

export interface PromptVersion {
	version: number;
	template?: string | null;
	messages?: { role: string; content: string }[] | null;
	config: Record<string, unknown>;
	labels: string[];
	commitMessage?: string | null;
}

export interface PromptSummary {
	name: string;
	type: string;
	versions: number;
	productionVersion?: number | null;
	labels: string[];
}

export interface PromptDetail {
	name: string;
	type: string;
	versions: PromptVersion[];
}

export async function listPrompts(base: string, t: string, projectId: string): Promise<PromptSummary[]> {
	const res = await fetch(`${base}/api/v1/projects/${projectId}/prompts`, { headers: authHeaders(t) });
	if (!res.ok) throw new Error(parseError(await res.text(), res.status));
	return (((await res.json()) as { data: PromptSummary[] }).data ?? []);
}

export async function getPromptDetail(base: string, t: string, projectId: string, name: string): Promise<PromptDetail> {
	const res = await fetch(`${base}/api/v1/projects/${projectId}/prompts/${name}`, {
		headers: authHeaders(t)
	});
	if (!res.ok) throw new Error(parseError(await res.text(), res.status));
	return (await res.json()) as PromptDetail;
}

export async function createPrompt(
	base: string,
	t: string,
	projectId: string,
	payload: { name: string; type: string; template?: string; messages?: { role: string; content: string }[] }
): Promise<void> {
	const res = await fetch(`${base}/api/v1/projects/${projectId}/prompts`, {
		method: 'POST',
		headers: { 'Content-Type': 'application/json', ...authHeaders(t) },
		body: JSON.stringify(payload)
	});
	if (!res.ok) throw new Error(parseError(await res.text(), res.status));
}

export async function createPromptVersion(
	base: string,
	t: string,
	projectId: string,
	name: string,
	payload: { template?: string; commitMessage?: string }
): Promise<void> {
	const res = await fetch(`${base}/api/v1/projects/${projectId}/prompts/${name}/versions`, {
		method: 'POST',
		headers: { 'Content-Type': 'application/json', ...authHeaders(t) },
		body: JSON.stringify(payload)
	});
	if (!res.ok) throw new Error(parseError(await res.text(), res.status));
}

export async function setPromptLabels(
	base: string,
	t: string,
	projectId: string,
	name: string,
	version: number,
	labels: string[]
): Promise<void> {
	const res = await fetch(`${base}/api/v1/projects/${projectId}/prompts/${name}/labels`, {
		method: 'POST',
		headers: { 'Content-Type': 'application/json', ...authHeaders(t) },
		body: JSON.stringify({ version, labels })
	});
	if (!res.ok) throw new Error(parseError(await res.text(), res.status));
}

export interface DatasetSummary {
	id: string;
	name: string;
	description?: string | null;
	itemCount: number;
	runCount: number;
}

export interface DatasetItem {
	id: string;
	input?: string | null;
	expectedOutput?: string | null;
}

export interface DatasetDetail extends DatasetSummary {
	items: DatasetItem[];
}

export interface DatasetRun {
	id: string;
	name: string;
	itemCount: number;
}

async function reqJSON(base: string, t: string, path: string, method = 'GET', payload?: unknown) {
	const res = await fetch(`${base}${path}`, {
		method,
		headers: { 'Content-Type': 'application/json', ...authHeaders(t) },
		body: payload === undefined ? undefined : JSON.stringify(payload)
	});
	if (!res.ok) throw new Error(parseError(await res.text(), res.status));
	return (await res.json()) as { data: never };
}

export async function listDatasets(base: string, t: string, projectId: string): Promise<DatasetSummary[]> {
	const out = await reqJSON(base, t, `/api/v1/projects/${projectId}/datasets`);
	return (out.data ?? []) as DatasetSummary[];
}

export async function createDataset(base: string, t: string, projectId: string, name: string): Promise<void> {
	await reqJSON(base, t, `/api/v1/projects/${projectId}/datasets`, 'POST', { name });
}

export async function getDataset(base: string, t: string, projectId: string, id: string): Promise<DatasetDetail> {
	const out = await reqJSON(base, t, `/api/v1/projects/${projectId}/datasets/${id}`);
	return out.data as DatasetDetail;
}

export async function addDatasetItem(
	base: string,
	t: string,
	projectId: string,
	datasetId: string,
	input: string,
	expectedOutput: string
): Promise<void> {
	await reqJSON(base, t, `/api/v1/projects/${projectId}/datasets/${datasetId}/items`, 'POST', {
		input,
		expectedOutput
	});
}

export async function listRuns(base: string, t: string, projectId: string, datasetId: string): Promise<DatasetRun[]> {
	const out = await reqJSON(base, t, `/api/v1/projects/${projectId}/datasets/${datasetId}/runs`);
	return (out.data ?? []) as DatasetRun[];
}

export async function createRun(base: string, t: string, projectId: string, datasetId: string, name: string): Promise<void> {
	await reqJSON(base, t, `/api/v1/projects/${projectId}/datasets/${datasetId}/runs`, 'POST', { name });
}

export interface RunDetail {
	id: string;
	name: string;
	items: { itemId: string; traceId?: string | null; traceName?: string | null }[];
}

export async function getRun(base: string, t: string, projectId: string, runId: string): Promise<RunDetail> {
	const out = await reqJSON(base, t, `/api/v1/projects/${projectId}/runs/${runId}`);
	return out.data as RunDetail;
}

export async function linkRunItem(
	base: string,
	t: string,
	projectId: string,
	runId: string,
	itemId: string,
	traceId: string
): Promise<void> {
	await reqJSON(base, t, `/api/v1/projects/${projectId}/runs/${runId}/items`, 'POST', { itemId, traceId });
}

export interface MetricsOverview {
	days: number;
	traces: number;
	observations: number;
	inputTokens: number;
	outputTokens: number;
	avgLatencyMs?: number | null;
	perDay: { date: string; traces: number; observations: number }[];
	byModel: { model: string; observations: number; inputTokens: number; outputTokens: number }[];
	truncated: boolean;
}

export async function getMetrics(base: string, t: string, projectId: string, days = 30): Promise<MetricsOverview> {
	const res = await fetch(`${base}/api/v1/projects/${projectId}/metrics/overview?days=${days}`, {
		headers: authHeaders(t)
	});
	if (!res.ok) throw new Error(parseError(await res.text(), res.status));
	return ((await res.json()) as { data: MetricsOverview }).data;
}

export interface PlaygroundResult {
	output: string;
	model: string;
	inputTokens: number;
	outputTokens: number;
	totalTokens: number;
	latencyMs: number;
	traceId?: string | null;
}

// runPlayground proxies a template through the backend. The provider key is
// request-scoped: it is never persisted server-side (nor in localStorage —
// kept in component state only).
export async function runPlayground(
	base: string,
	t: string,
	projectId: string,
	payload: {
		template?: string;
		promptName?: string;
		variables: Record<string, string>;
		provider: { baseUrl: string; apiKey: string; model: string };
		saveAsTrace?: boolean;
	}
): Promise<PlaygroundResult> {
	const res = await fetch(`${base}/api/v1/projects/${projectId}/playground/run`, {
		method: 'POST',
		headers: { 'Content-Type': 'application/json', ...authHeaders(t) },
		body: JSON.stringify(payload)
	});
	if (!res.ok) throw new Error(parseError(await res.text(), res.status));
	return ((await res.json()) as { data: PlaygroundResult }).data;
}

export interface Session {
	sessionId: string;
	traceCount: number;
	lastTraceAt: string;
}

export async function listSessions(base: string, t: string, projectId: string): Promise<Session[]> {
	const res = await fetch(`${base}/api/v1/projects/${projectId}/sessions`, { headers: authHeaders(t) });
	if (!res.ok) throw new Error(parseError(await res.text(), res.status));
	return (((await res.json()) as { data: Session[] }).data ?? []);
}

export interface ScoreRow {
	traceId: string;
	observationId?: string | null;
	name: string;
	value: number | string | boolean | null;
	dataType: string;
	comment?: string | null;
}

export async function listScores(
	base: string,
	t: string,
	projectId: string,
	params: { traceId?: string; name?: string } = {}
): Promise<ScoreRow[]> {
	const q = new URLSearchParams();
	if (params.traceId) q.set('traceId', params.traceId);
	if (params.name) q.set('name', params.name);
	const res = await fetch(`${base}/api/v1/projects/${projectId}/scores?${q}`, { headers: authHeaders(t) });
	if (!res.ok) throw new Error(parseError(await res.text(), res.status));
	return (((await res.json()) as { data: ScoreRow[] }).data ?? []);
}

export interface ScoreConfig {
	id: string;
	name: string;
	dataType: string;
	minValue?: number | null;
	maxValue?: number | null;
	categories: string[];
}

export async function listScoreConfigs(base: string, t: string, projectId: string): Promise<ScoreConfig[]> {
	const res = await fetch(`${base}/api/v1/projects/${projectId}/score-configs`, { headers: authHeaders(t) });
	if (!res.ok) throw new Error(parseError(await res.text(), res.status));
	return (((await res.json()) as { data: ScoreConfig[] }).data ?? []);
}

export async function createScoreConfig(
	base: string,
	t: string,
	projectId: string,
	payload: { name: string; dataType: string; minValue?: number; maxValue?: number; categories?: string[] }
): Promise<void> {
	const res = await fetch(`${base}/api/v1/projects/${projectId}/score-configs`, {
		method: 'POST',
		headers: { 'Content-Type': 'application/json', ...authHeaders(t) },
		body: JSON.stringify(payload)
	});
	if (!res.ok) throw new Error(parseError(await res.text(), res.status));
}

export interface ReviewQueue {
	id: string;
	name: string;
	pending: number;
	total: number;
}

export interface QueueItem {
	id: string;
	traceId?: string | null;
	observationId?: string | null;
	status: string;
	traceName?: string | null;
}

export async function listQueues(base: string, t: string, projectId: string): Promise<ReviewQueue[]> {
	const res = await fetch(`${base}/api/v1/projects/${projectId}/annotation-queues`, {
		headers: authHeaders(t)
	});
	if (!res.ok) throw new Error(parseError(await res.text(), res.status));
	return (((await res.json()) as { data: ReviewQueue[] }).data ?? []);
}

export async function createQueue(base: string, t: string, projectId: string, name: string): Promise<void> {
	await reqJSON(base, t, `/api/v1/projects/${projectId}/annotation-queues`, 'POST', { name });
}

export async function getQueue(
	base: string,
	t: string,
	projectId: string,
	queueId: string,
	status?: string
): Promise<QueueItem[]> {
	const q = status ? `?status=${status}` : '';
	const res = await fetch(`${base}/api/v1/projects/${projectId}/annotation-queues/${queueId}${q}`, {
		headers: authHeaders(t)
	});
	if (!res.ok) throw new Error(parseError(await res.text(), res.status));
	const body = (await res.json()) as { data: { items: QueueItem[] } };
	return body.data.items ?? [];
}

export async function addQueueItem(base: string, t: string, projectId: string, queueId: string, traceId: string): Promise<void> {
	await reqJSON(base, t, `/api/v1/projects/${projectId}/annotation-queues/${queueId}/items`, 'POST', { traceId });
}

export async function completeQueueItem(base: string, t: string, projectId: string, queueId: string, itemId: string): Promise<void> {
	await reqJSON(base, t, `/api/v1/projects/${projectId}/annotation-queues/${queueId}/items/${itemId}/complete`, 'POST', {});
}

export async function scoreQueueItem(
	base: string,
	t: string,
	projectId: string,
	queueId: string,
	itemId: string,
	name: string,
	value: string
): Promise<void> {
	const num = Number(value);
	await reqJSON(
		base,
		t,
		`/api/v1/projects/${projectId}/annotation-queues/${queueId}/items/${itemId}/scores`,
		'POST',
		{ name, value: value !== '' && !Number.isNaN(num) ? num : value }
	);
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
