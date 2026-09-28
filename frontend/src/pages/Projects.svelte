<script lang="ts">
	import { onMount } from 'svelte';
	import { API_BASE, createProject, currentOrgId, listProjects, setProject, token, type Project } from '../lib/api';
	import { get } from 'svelte/store';

	let projects = $state<Project[]>([]);
	let error = $state('');
	let name = $state('');
	let creating = $state(false);

	async function load() {
		error = '';
		try {
			projects = await listProjects(API_BASE, get(token) ?? '');
		} catch (err) {
			error = err instanceof Error ? err.message : 'failed to load';
		}
	}

	async function create(e: Event) {
		e.preventDefault();
		creating = true;
		error = '';
		try {
			// Server defaults to the caller's org when omitted (single-org
			// accounts); stored org is sent when we know it (multi-org).
			await createProject(API_BASE, get(token) ?? '', name, get(currentOrgId) ?? undefined);
			name = '';
			await load();
		} catch (err) {
			error = err instanceof Error ? err.message : 'create failed';
		} finally {
			creating = false;
		}
	}

	function select(id: string) {
		setProject(id);
		location.hash = '#/traces';
	}

	onMount(load);
</script>

<div class="card bg-base-100 shadow">
	<div class="card-body">
		<h2 class="card-title">Projects</h2>
		{#if error}<div class="alert alert-error text-sm">{error}</div>{/if}
		{#if projects.length === 0}
			<p class="text-sm opacity-70">No projects yet — create one to get API keys for SDK ingestion.</p>
		{:else}
			<ul class="menu bg-base-200 rounded-box">
				{#each projects as p}
					<li><button onclick={() => select(p.id)}>{p.name}</button></li>
				{/each}
			</ul>
		{/if}
		<form class="flex gap-2 mt-4" onsubmit={create}>
			<input class="input input-bordered flex-1" placeholder="New project name" bind:value={name} required />
			<button class="btn btn-primary" disabled={creating}>Create</button>
		</form>
	</div>
</div>
