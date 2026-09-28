import { describe, expect, it } from 'vitest';
import { authHeaders, formatLatency, parseError, tracesListUrl, tracesUrl } from './api';

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
