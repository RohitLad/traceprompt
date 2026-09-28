<script lang="ts">
	import { onMount } from 'svelte';
	import { get } from 'svelte/store';
	import {
		API_BASE,
		currentProjectId,
		fetchHealth,
		formatLatency,
		getMetrics,
		listProjects,
		setProject,
		token,
		type MetricsOverview,
		type Project
	} from '../lib/api';

	let projects = $state<Project[]>([]);
	let projectId = $state('');
	let backend = $state('checking…');
	let metrics = $state<MetricsOverview | null>(null);
	let error = $state('');

	async function load(withProjects = true) {
		error = '';
		try {
			backend = (await fetchHealth()).service;
			if (withProjects || projects.length === 0) {
				projects = await listProjects(API_BASE, get(token) ?? '');
				projectId = get(currentProjectId) ?? projects[0]?.id ?? '';
				setProject(projectId);
			}
			if (projectId) metrics = await getMetrics(API_BASE, get(token) ?? '', projectId);
		} catch (err) {
			error = err instanceof Error ? err.message : 'failed to load';
		}
	}

	function maxObs(): number {
		if (!metrics || metrics.perDay.length === 0) return 1;
		return Math.max(...metrics.perDay.map((d) => d.observations), 1);
	}

	onMount(() => load());
</script>

<div class="flex items-center justify-between flex-wrap gap-2 mb-4">
	<h1 class="text-xl font-bold">Dashboard <span class="text-sm font-normal opacity-60">({backend})</span></h1>
	<select class="select select-bordered select-sm max-w-[12rem]" bind:value={projectId} onchange={() => load(false)}>
		{#each projects as p}<option value={p.id}>{p.name}</option>{/each}
	</select>
</div>

{#if error}<div class="alert alert-error text-sm mb-2">{error}</div>{/if}
{#if !metrics}
	<div class="flex justify-center py-8"><span class="loading loading-spinner"></span></div>
{:else}
	<div class="stats shadow stats-vertical sm:stats-horizontal w-full">
		<div class="stat"><div class="stat-title">Traces</div><div class="stat-value text-2xl">{metrics.traces}</div></div>
		<div class="stat"><div class="stat-title">Observations</div><div class="stat-value text-2xl">{metrics.observations}</div></div>
		<div class="stat"><div class="stat-title">Tokens in/out</div><div class="stat-value text-2xl">{metrics.inputTokens}/{metrics.outputTokens}</div></div>
		<div class="stat"><div class="stat-title">Avg latency</div><div class="stat-value text-2xl">{formatLatency(metrics.avgLatencyMs)}</div></div>
	</div>
	{#if metrics.truncated}<p class="text-xs opacity-60 mt-1">Large project — aggregates capped, use blob export for exact numbers.</p>{/if}

	<div class="grid md:grid-cols-2 gap-4 mt-4">
		<div class="card bg-base-100 shadow">
			<div class="card-body">
				<h2 class="card-title text-base">Observations per day</h2>
				{#if metrics.perDay.length === 0}
					<p class="text-sm opacity-60">No data in range.</p>
				{:else}
					<div class="flex items-end gap-1 h-32">
						{#each metrics.perDay as d}
							<div class="flex-1 flex flex-col items-center gap-1" title="{d.date}: {d.observations} obs, {d.traces} traces">
								<div class="bg-primary w-full rounded-t" style="height: {(d.observations / maxObs()) * 100}%"></div>
								<span class="text-[10px] opacity-60">{d.date.slice(5)}</span>
							</div>
						{/each}
					</div>
				{/if}
			</div>
		</div>
		<div class="card bg-base-100 shadow">
			<div class="card-body">
				<h2 class="card-title text-base">By model</h2>
				<table class="table table-sm">
					<thead><tr><th>Model</th><th class="text-right">Obs</th><th class="text-right">Tokens</th></tr></thead>
					<tbody>
						{#each metrics.byModel as m}
							<tr><td class="text-sm">{m.model}</td><td class="text-right">{m.observations}</td><td class="text-right">{m.inputTokens + m.outputTokens}</td></tr>
						{/each}
					</tbody>
				</table>
				{#if metrics.byModel.length === 0}<p class="text-sm opacity-60">No data in range.</p>{/if}
			</div>
		</div>
	</div>
{/if}
