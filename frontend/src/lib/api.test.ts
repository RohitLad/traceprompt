import { describe, expect, it } from 'vitest';
import { formatLatency, tracesUrl } from './api';

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
