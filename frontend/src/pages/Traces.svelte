<script lang="ts">
	import { onMount } from 'svelte';
	import { get } from 'svelte/store';
	import {
		API_BASE,
		currentProjectId,
		listProjects,
		listTraces,
		setProject,
		token,
		type Project,
		type UITrace
	} from '../lib/api';

	let projects = $state<Project[]>([]);
	let projectId = $state('');
	let traces = $state<UITrace[]>([]);
	let cursor = $state<string | null>(null);
	// Page cursors: pages[i] is the cursor argument that loads page i.
	// Index 0 (undefined) is the first page.
	let pages = $state<(string | undefined)[]>([undefined]);
	let page = $state(0);
	let error = $state('');
	let loading = $state(true);

	async function loadProjects() {
		projects = await listProjects(API_BASE, get(token) ?? '');
		projectId = get(currentProjectId) ?? projects[0]?.id ?? '';
	}

	async function load() {
		loading = true;
		error = '';
		try {
			if (!projectId) await loadProjects();
			if (!projectId) {
				error = 'No projects yet — create one first.';
				return;
			}
			setProject(projectId);
			const out = await listTraces(API_BASE, get(token) ?? '', projectId, 25, pages[page]);
			traces = out.data;
			cursor = out.cursor;
		} catch (err) {
			error = err instanceof Error ? err.message : 'failed to load';
		} finally {
			loading = false;
		}
	}

	function reset() {
		pages = [undefined];
		page = 0;
		load();
	}

	function next() {
		if (!cursor) return;
		pages = [...pages.slice(0, page + 1), cursor];
		page += 1;
		load();
	}

	function prev() {
		if (page === 0) return;
		page -= 1;
		load();
	}

	onMount(() => load());
</script>

<div class="card bg-base-100 shadow">
	<div class="card-body">
		<div class="flex items-center justify-between flex-wrap gap-2">
			<h2 class="card-title">Traces</h2>
			<select
			 class="select select-bordered select-sm max-w-xs"
				bind:value={projectId}
				onchange={reset}
			>
				{#each projects as p}<option value={p.id}>{p.name}</option>{/each}
			</select>
		</div>
		{#if error}<div class="alert alert-error text-sm">{error}</div>{/if}
		{#if loading}
			<div class="flex justify-center py-8"><span class="loading loading-spinner"></span></div>
		{:else}
			<div class="overflow-x-auto">
				<table class="table table-zebra">
					<thead><tr><th>Trace</th><th>User</th><th>Session</th><th class="text-right">Obs</th></tr></thead>
					<tbody>
						{#if traces.length === 0}
							<tr><td colspan="4" class="text-center opacity-60">No traces yet — send your first ingestion batch.</td></tr>
						{:else}
							{#each traces as t}
								<tr>
									<td>
										<a class="link font-mono text-xs" href="#/traces/{t.traceId}">{t.traceId}</a>
										<div class="text-sm">{t.name || '—'}</div>
									</td>
									<td class="text-sm">{t.userId ?? '—'}</td>
									<td class="font-mono text-xs">{t.sessionId ?? '—'}</td>
									<td class="text-right">{t.observationCount}</td>
								</tr>
							{/each}
						{/if}
					</tbody>
				</table>
			</div>
			<div class="join justify-end mt-2">
				<button class="join-item btn btn-sm" disabled={page === 0} onclick={prev}>« Prev</button>
				<button class="join-item btn btn-sm" disabled={!cursor} onclick={next}>Next »</button>
			</div>
		{/if}
	</div>
</div>
