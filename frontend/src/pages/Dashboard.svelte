<script lang="ts">
	import { onMount } from 'svelte';
	import { fetchHealth } from '../lib/api';

	let status = $state('checking…');
	let healthy = $state(false);

	onMount(async () => {
		try {
			const h = await fetchHealth();
			healthy = h.status === 'ok';
			status = healthy ? `connected (${h.service})` : 'unexpected response';
		} catch {
			status = 'backend unreachable — start it with `docker compose up api`';
		}
	});
</script>

<div class="stats shadow">
	<div class="stat">
		<div class="stat-title">Backend</div>
		<div class="stat-value text-lg">{status}</div>
		<div class="stat-desc">Langfuse-compatible ingestion: Phase 2</div>
	</div>
	<div class="stat">
		<div class="stat-title">Traces (24h)</div>
		<div class="stat-value text-lg">—</div>
		<div class="stat-desc">Metrics API lands in Phase 5</div>
	</div>
	<div class="stat">
		<div class="stat-title">Avg latency</div>
		<div class="stat-value text-lg">—</div>
		<div class="stat-desc">No TanStack — plain tables + server pagination</div>
	</div>
</div>

<div class="card bg-base-100 shadow mt-4">
	<div class="card-body">
		<h2 class="card-title">Getting started</h2>
		<ol class="list-decimal ml-6 space-y-1 text-sm">
			<li>Start stack: <code>docker compose up --build</code></li>
			<li>Open API health: <code>/api/health</code></li>
			<li>Phase 2 will light up Traces with real ingestion data.</li>
		</ol>
	</div>
</div>
