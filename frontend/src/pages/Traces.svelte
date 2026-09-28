<script lang="ts">
	// Server-paginated table on purpose: no client data-grid lib (no TanStack).
	// Phase 2 wires this to GET /api/public/v2/observations.
	let rows = $state<{ traceId: string; name: string; type: string }[]>([]);
	let notice =
		$state('No data yet — ingestion API lands in Phase 2. This table already supports server pagination.');
</script>

<div class="card bg-base-100 shadow">
	<div class="card-body">
		<div class="flex items-center justify-between">
			<h2 class="card-title">Traces</h2>
			<div class="join">
				<button class="join-item btn btn-sm" disabled>« Prev</button>
				<button class="join-item btn btn-sm" disabled>Next »</button>
			</div>
		</div>
		<p class="text-sm opacity-70">{notice}</p>
		<div class="overflow-x-auto">
			<table class="table table-zebra">
				<thead>
					<tr><th>Trace ID</th><th>Name</th><th>Type</th></tr>
				</thead>
				<tbody>
					{#if rows.length === 0}
						<tr><td colspan="3" class="text-center opacity-60">Empty — send your first trace to see it here.</td></tr>
					{:else}
						{#each rows as r}
							<tr><td class="font-mono text-xs">{r.traceId}</td><td>{r.name}</td><td>{r.type}</td></tr>
						{/each}
					{/if}
				</tbody>
			</table>
		</div>
	</div>
</div>
