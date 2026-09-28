<script lang="ts">
	import { onMount } from 'svelte';
	import { get } from 'svelte/store';
	import {
		API_BASE,
		currentProjectId,
		listProjects,
		listScores,
		setProject,
		token,
		type Project,
		type ScoreRow
	} from '../lib/api';

	let projects = $state<Project[]>([]);
	let projectId = $state('');
	let scores = $state<ScoreRow[]>([]);
	let traceFilter = $state('');
	let nameFilter = $state('');
	let error = $state('');

	async function load() {
		error = '';
		try {
			projects = await listProjects(API_BASE, get(token) ?? '');
			projectId = get(currentProjectId) ?? projects[0]?.id ?? '';
			setProject(projectId);
			if (!projectId) {
				error = 'No projects yet.';
				return;
			}
			scores = await listScores(API_BASE, get(token) ?? '', projectId, {
				traceId: traceFilter || undefined,
				name: nameFilter || undefined
			});
		} catch (err) {
			error = err instanceof Error ? err.message : 'failed to load';
		}
	}

	function valueText(s: ScoreRow): string {
		if (s.value == null) return '—';
		return String(s.value);
	}

	onMount(load);
</script>

<div class="card bg-base-100 shadow">
	<div class="card-body">
		<div class="flex items-center justify-between flex-wrap gap-2">
			<h2 class="card-title">Scores</h2>
			<select class="select select-bordered select-sm max-w-[10rem]" bind:value={projectId} onchange={load}>
				{#each projects as p}<option value={p.id}>{p.name}</option>{/each}
			</select>
		</div>
		{#if error}<div class="alert alert-error text-sm">{error}</div>{/if}
		<form class="flex gap-2" onsubmit={(e) => { e.preventDefault(); load(); }}>
			<input class="input input-bordered input-sm flex-1" placeholder="filter: trace id" bind:value={traceFilter} />
			<input class="input input-bordered input-sm flex-1" placeholder="filter: score name" bind:value={nameFilter} />
			<button class="btn btn-sm">Filter</button>
		</form>
		<div class="overflow-x-auto">
			<table class="table table-zebra table-sm">
				<thead><tr><th>Trace</th><th>Name</th><th>Value</th><th>Type</th></tr></thead>
				<tbody>
					{#if scores.length === 0}
						<tr><td colspan="4" class="text-center opacity-60">No scores yet — add them via SDK, API, or ingestion.</td></tr>
					{:else}
						{#each scores as s}
							<tr>
								<td><a class="link font-mono text-xs" href="#/traces/{s.traceId}">{s.traceId.slice(0, 12)}…</a></td>
								<td class="text-sm">{s.name}</td>
								<td><span class="badge badge-sm">{valueText(s)}</span></td>
								<td class="text-xs opacity-60">{s.dataType}</td>
							</tr>
						{/each}
					{/if}
				</tbody>
			</table>
		</div>
	</div>
</div>
