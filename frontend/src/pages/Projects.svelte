<script lang="ts">
	import { onMount } from 'svelte';
	import { API_BASE, authHeaders, listProjects, setProject, token, type Project } from '../lib/api';
	import { get } from 'svelte/store';

	let projects = $state<Project[]>([]);
	let error = $state('');
	let name = $state('');
	let orgId = $state('');
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
			const t = get(token) ?? '';
			// orgId defaults to the first project's org for convenience.
			const org = orgId || projects[0]?.organizationId;
			if (!org) throw new Error('no organization — register an account first');
			const res = await fetch(`${API_BASE}/api/v1/projects`, {
				method: 'POST',
				headers: { 'Content-Type': 'application/json', ...authHeaders(t) },
				body: JSON.stringify({ name, organizationId: org })
			});
			if (!res.ok) throw new Error('create failed');
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
