<script lang="ts">
	import { onMount } from 'svelte';
	import { get } from 'svelte/store';
	import {
		API_BASE,
		currentProjectId,
		listDatasets,
		listProjects,
		createDataset,
		setProject,
		token,
		type DatasetSummary,
		type Project
	} from '../lib/api';

	let projects = $state<Project[]>([]);
	let projectId = $state('');
	let datasets = $state<DatasetSummary[]>([]);
	let name = $state('');
	let error = $state('');

	async function load() {
		error = '';
		try {
			projects = await listProjects(API_BASE, get(token) ?? '');
			projectId = get(currentProjectId) ?? projects[0]?.id ?? '';
			setProject(projectId);
			if (!projectId) {
				error = 'No projects yet — create one first.';
				return;
			}
			datasets = await listDatasets(API_BASE, get(token) ?? '', projectId);
		} catch (err) {
			error = err instanceof Error ? err.message : 'failed to load';
		}
	}

	async function create(e: Event) {
		e.preventDefault();
		try {
			await createDataset(API_BASE, get(token) ?? '', projectId, name);
			name = '';
			await load();
		} catch (err) {
			error = err instanceof Error ? err.message : 'create failed';
		}
	}

	onMount(load);
</script>

<div class="card bg-base-100 shadow">
	<div class="card-body">
		<div class="flex items-center justify-between flex-wrap gap-2">
			<h2 class="card-title">Datasets</h2>
			<select class="select select-bordered select-sm max-w-[10rem]" bind:value={projectId} onchange={load}>
				{#each projects as p}<option value={p.id}>{p.name}</option>{/each}
			</select>
		</div>
		{#if error}<div class="alert alert-error text-sm">{error}</div>{/if}
		<ul class="menu bg-base-200 rounded-box">
			{#each datasets as d}
				<li>
					<a href="#/datasets/{d.id}">
						{d.name}
						<span class="badge badge-sm">{d.itemCount} items</span>
						<span class="badge badge-sm">{d.runCount} runs</span>
					</a>
				</li>
			{/each}
		</ul>
		{#if datasets.length === 0}<p class="text-sm opacity-60">No datasets yet — create a test set for experiments.</p>{/if}
		<form class="flex gap-2 mt-2" onsubmit={create}>
			<input class="input input-bordered flex-1 input-sm" placeholder="New dataset name" bind:value={name} required />
			<button class="btn btn-primary btn-sm">Create</button>
		</form>
	</div>
</div>
