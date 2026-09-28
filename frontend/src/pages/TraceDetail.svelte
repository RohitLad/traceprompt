<script lang="ts">
	import { onMount } from 'svelte';
	import { get } from 'svelte/store';
	import {
		API_BASE,
		currentProjectId,
		formatLatency,
		getTrace,
		listProjects,
		token,
		type UIObservation
	} from '../lib/api';

	let { params } = $props<{ params: { traceId: string } }>();

	let name = $state('');
	let observations = $state<UIObservation[]>([]);
	let expanded = $state<string | null>(null);
	let error = $state('');
	let loading = $state(true);

	function latency(o: UIObservation): number | null {
		if (!o.startTime || !o.endTime) return null;
		return new Date(o.endTime).getTime() - new Date(o.startTime).getTime();
	}

	onMount(async () => {
		try {
			let pid = get(currentProjectId);
			if (!pid) {
				const projects = await listProjects(API_BASE, get(token) ?? '');
				pid = projects[0]?.id ?? '';
			}
			if (!pid) throw new Error('No projects yet.');
			const data = await getTrace(API_BASE, get(token) ?? '', pid, params.traceId);
			name = data.name;
			observations = data.observations ?? [];
		} catch (err) {
			error = err instanceof Error ? err.message : 'failed to load';
		} finally {
			loading = false;
		}
	});
</script>

<a class="btn btn-ghost btn-sm mb-2" href="#/traces">← Traces</a>
<div class="card bg-base-100 shadow">
	<div class="card-body">
		<h2 class="card-title font-mono text-sm">{params.traceId}</h2>
		<p class="text-sm opacity-70">{name || '—'} · {observations.length} observations</p>
		{#if error}<div class="alert alert-error text-sm">{error}</div>{/if}
		{#if loading}
			<div class="flex justify-center py-8"><span class="loading loading-spinner"></span></div>
		{:else}
			<ul class="timeline timeline-vertical">
				{#each observations as o}
					<li>
						<div class="timeline-start text-xs opacity-60">
							{new Date(o.startTime).toLocaleTimeString()}
						</div>
						<div class="timeline-middle">
							<div class="badge badge-sm">{o.type}</div>
						</div>
						<div class="timeline-end timeline-box w-full">
							<div class="flex items-center justify-between gap-2">
								<span class="font-medium text-sm">{o.name || o.type}</span>
								<span class="text-xs opacity-60">{formatLatency(latency(o))}</span>
							</div>
							{#if o.model}<div class="text-xs opacity-60">model: {o.model}</div>{/if}
							<button
								class="btn btn-ghost btn-xs mt-1"
								onclick={() => (expanded = expanded === o.id ? null : o.id)}
							>
								{expanded === o.id ? 'Hide I/O' : 'Show I/O'}
							</button>
							{#if expanded === o.id}
								<pre class="text-xs bg-base-200 rounded p-2 mt-1 overflow-x-auto">in:  {o.input ?? '—'}\nout: {o.output ?? '—'}</pre>
							{/if}
						</div>
						<hr />
					</li>
				{/each}
			</ul>
			{#if observations.length === 0 && !error}
				<p class="text-sm opacity-60">No observations in this trace.</p>
			{/if}
		{/if}
	</div>
</div>
