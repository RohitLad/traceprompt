<script lang="ts">
	import { onMount } from 'svelte';
	import { get } from 'svelte/store';
	import {
		API_BASE,
		addDatasetItem,
		createRun,
		currentProjectId,
		getDataset,
		getRun,
		linkRunItem,
		listProjects,
		listRuns,
		token,
		type DatasetDetail,
		type DatasetRun,
		type RunDetail
	} from '../lib/api';

	let { params } = $props<{ params: { id: string } }>();

	let detail = $state<DatasetDetail | null>(null);
	let runs = $state<DatasetRun[]>([]);
	let selectedRun = $state<RunDetail | null>(null);
	let error = $state('');
	let loading = $state(true);

	let itemInput = $state('');
	let itemExpected = $state('');
	let runName = $state('');
	let linkItem = $state('');
	let linkTrace = $state('');

	async function projectId(): Promise<string> {
		let pid = get(currentProjectId);
		if (!pid) {
			const projects = await listProjects(API_BASE, get(token) ?? '');
			pid = projects[0]?.id ?? '';
		}
		if (!pid) throw new Error('No projects yet.');
		return pid;
	}

	async function load() {
		loading = true;
		error = '';
		try {
			const pid = await projectId();
			const t = get(token) ?? '';
			detail = await getDataset(API_BASE, t, pid, params.id);
			runs = await listRuns(API_BASE, t, pid, params.id);
			if (selectedRun) selectedRun = await getRun(API_BASE, t, pid, selectedRun.id);
		} catch (err) {
			error = err instanceof Error ? err.message : 'failed to load';
		} finally {
			loading = false;
		}
	}

	async function addItem(e: Event) {
		e.preventDefault();
		try {
			await addDatasetItem(API_BASE, get(token) ?? '', await projectId(), params.id, itemInput, itemExpected);
			itemInput = '';
			itemExpected = '';
			await load();
		} catch (err) {
			error = err instanceof Error ? err.message : 'add failed';
		}
	}

	async function addRun(e: Event) {
		e.preventDefault();
		try {
			await createRun(API_BASE, get(token) ?? '', await projectId(), params.id, runName);
			runName = '';
			await load();
		} catch (err) {
			error = err instanceof Error ? err.message : 'run failed';
		}
	}

	async function link(e: Event) {
		e.preventDefault();
		if (!selectedRun) return;
		try {
			await linkRunItem(API_BASE, get(token) ?? '', await projectId(), selectedRun.id, linkItem, linkTrace);
			linkItem = '';
			linkTrace = '';
			await load();
		} catch (err) {
			error = err instanceof Error ? err.message : 'link failed';
		}
	}

	onMount(load);
</script>

<a class="btn btn-ghost btn-sm mb-2" href="#/datasets">← Datasets</a>
{#if error}<div class="alert alert-error text-sm mb-2">{error}</div>{/if}
{#if loading}
	<div class="flex justify-center py-8"><span class="loading loading-spinner"></span></div>
{:else if !detail}
	<p class="text-sm opacity-60">Dataset not found.</p>
{:else}
	<div class="grid md:grid-cols-2 gap-4">
		<div class="card bg-base-100 shadow">
			<div class="card-body">
				<h2 class="card-title">{detail.name} <span class="badge">{detail.items.length} items</span></h2>
				<div class="overflow-x-auto">
					<table class="table table-zebra table-sm">
						<thead><tr><th>Input</th><th>Expected</th></tr></thead>
						<tbody>
							{#each detail.items as it}
								<tr><td class="text-xs max-w-[12rem] truncate">{it.input ?? '—'}</td><td class="text-xs max-w-[12rem] truncate">{it.expectedOutput ?? '—'}</td></tr>
							{/each}
						</tbody>
					</table>
				</div>
				<form class="space-y-2 mt-2" onsubmit={addItem}>
					<input class="input input-bordered w-full input-sm" placeholder="Input" bind:value={itemInput} required />
					<input class="input input-bordered w-full input-sm" placeholder="Expected output" bind:value={itemExpected} />
					<button class="btn btn-primary btn-sm">Add item</button>
				</form>
			</div>
		</div>

		<div class="card bg-base-100 shadow">
			<div class="card-body">
				<h2 class="card-title">Runs</h2>
				<ul class="menu bg-base-200 rounded-box">
					{#each runs as r}
						<li>
							<button
								class:active={selectedRun?.id === r.id}
								onclick={async () => {
									selectedRun = await getRun(API_BASE, get(token) ?? '', await projectId(), r.id);
								}}
							>
								{r.name} <span class="badge badge-sm">{r.itemCount}</span>
							</button>
						</li>
					{/each}
				</ul>
				<form class="flex gap-2 mt-2" onsubmit={addRun}>
					<input class="input input-bordered flex-1 input-sm" placeholder="New run name" bind:value={runName} required />
					<button class="btn btn-secondary btn-sm">Start run</button>
				</form>
				{#if selectedRun}
					<div class="divider">Linked traces</div>
					<ul class="space-y-1 text-sm">
						{#each selectedRun.items as li}
							<li>
								<span class="font-mono text-xs">{li.itemId.slice(0, 8)}</span> →
								{#if li.traceId}<a class="link font-mono text-xs" href="#/traces/{li.traceId}">{li.traceName ?? li.traceId}</a>
								{:else}<span class="opacity-60">no trace</span>{/if}
							</li>
						{/each}
					</ul>
					<form class="flex gap-2 mt-2" onsubmit={link}>
						<select class="select select-bordered select-sm flex-1" bind:value={linkItem} required>
							<option value="">item…</option>
							{#each detail.items as it}<option value={it.id}>{it.input?.slice(0, 24) ?? it.id.slice(0, 8)}</option>{/each}
						</select>
						<input class="input input-bordered input-sm flex-1" placeholder="trace id" bind:value={linkTrace} required />
						<button class="btn btn-sm">Link</button>
					</form>
				{/if}
			</div>
		</div>
	</div>
{/if}
