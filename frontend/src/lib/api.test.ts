import { beforeEach, describe, expect, it, vi } from 'vitest';
import { authHeaders, formatLatency, getMetrics, listProjects, login, parseError, tracesListUrl, tracesUrl } from './api';

describe('tracesUrl', () => {
	it('builds base observations URL without params', () => {
		expect(tracesUrl()).toContain('/api/public/v2/observations');
	});

	it('appends limit and cursor', () => {
		const url = tracesUrl({ limit: 50, cursor: 'abc' });
		expect(url).toContain('limit=50');
		expect(url).toContain('cursor=abc');
	});
});

describe('formatLatency', () => {
	it('formats ms and seconds', () => {
		expect(formatLatency(null)).toBe('—');
		expect(formatLatency(42)).toBe('42ms');
		expect(formatLatency(1500)).toBe('1.50s');
	});
});

describe('authHeaders', () => {
	it('returns bearer header or empty', () => {
		expect(authHeaders('tok')).toEqual({ Authorization: 'Bearer tok' });
		expect(authHeaders(null)).toEqual({});
	});
});

describe('parseError', () => {
	it('extracts server error message', () => {
		expect(parseError('{"error":"bad email"}', 400)).toBe('bad email');
	});

	it('falls back to status', () => {
		expect(parseError('not json', 500)).toBe('request failed (500)');
	});
});

describe('tracesListUrl', () => {
	it('builds UI traces URL with cursor', () => {
		const url = tracesListUrl('http://api', 'pid-1', 10, 'cur');
		expect(url).toBe('http://api/api/v1/projects/pid-1/traces?limit=10&cursor=cur');
	});
});

function mockFetchOnce(status: number, body: unknown) {
	return vi.stubGlobal(
		'fetch',
		vi.fn().mockResolvedValue({
			ok: status >= 200 && status < 300,
			status,
			text: () => Promise.resolve(typeof body === 'string' ? body : JSON.stringify(body)),
			json: () => Promise.resolve(body)
		})
	);
}

describe('fetch wrappers', () => {
	beforeEach(() => {
		vi.unstubAllGlobals();
	});

	it('login posts credentials and returns token', async () => {
		mockFetchOnce(200, { token: 'tok', user: {}, org: { id: 'o', name: 'O' } });
		const out = await login('http://api', 'a@b.c', 'password-123');
		expect(out.token).toBe('tok');
		expect(fetch).toHaveBeenCalledWith(
			'http://api/api/v1/auth/login',
			expect.objectContaining({ method: 'POST' })
		);
	});

	it('login surfaces server error message', async () => {
		mockFetchOnce(401, 'ignored');
		vi.stubGlobal(
			'fetch',
			vi.fn().mockResolvedValue({
				ok: false,
				status: 401,
				text: () => Promise.resolve('{"error":"invalid credentials"}'),
				json: () => Promise.resolve({})
			})
		);
		await expect(login('http://api', 'a@b.c', 'wrong')).rejects.toThrow('invalid credentials');
	});

	it('listProjects sends bearer token and unwraps data', async () => {
		mockFetchOnce(200, { data: [{ id: 'p', name: 'P', organizationId: 'o' }] });
		const out = await listProjects('http://api', 'tok');
		expect(out).toHaveLength(1);
		expect(fetch).toHaveBeenCalledWith(
			'http://api/api/v1/projects',
			expect.objectContaining({ headers: { Authorization: 'Bearer tok' } })
		);
	});

	it('getMetrics throws with status on failure', async () => {
		mockFetchOnce(403, {});
		vi.stubGlobal(
			'fetch',
			vi.fn().mockResolvedValue({
				ok: false,
				status: 403,
				text: () => Promise.resolve('forbidden'),
				json: () => Promise.resolve({})
			})
		);
		await expect(getMetrics('http://api', 'tok', 'p')).rejects.toThrow('request failed (403)');
	});
});
