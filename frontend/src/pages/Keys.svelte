<script lang="ts">
	import { onMount } from 'svelte';
	import { get } from 'svelte/store';
	import {
		API_BASE,
		createKey,
		currentProjectId,
		listKeys,
		listProjects,
		revokeKey,
		token,
		type APIKey,
		type Project
	} from '../lib/api';

	let projects = $state<Project[]>([]);
	let projectId = $state('');
	let keys = $state<APIKey[]>([]);
	let keyName = $state('');
	let newSecret = $state('');
	let error = $state('');

	async function load() {
		error = '';
		try {
			projects = await listProjects(API_BASE, get(token) ?? '');
			projectId = get(currentProjectId) ?? projects[0]?.id ?? '';
			if (projectId) await loadKeys();
			else keys = [];
		} catch (err) {
			error = err instanceof Error ? err.message : 'failed to load';
		}
	}

	async function loadKeys() {
		try {
			keys = await listKeys(API_BASE, get(token) ?? '', projectId);
		} catch (err) {
			error = err instanceof Error ? err.message : 'failed to load keys';
		}
	}

	async function create(e: Event) {
		e.preventDefault();
		error = '';
		newSecret = '';
		try {
			const created = await createKey(API_BASE, get(token) ?? '', projectId, keyName);
			newSecret = created.secret;
			keyName = '';
			await loadKeys();
		} catch (err) {
			error = err instanceof Error ? err.message : 'create failed';
		}
	}

	async function revoke(id: string) {
		error = '';
		try {
			await revokeKey(API_BASE, get(token) ?? '', projectId, id);
			await loadKeys();
		} catch (err) {
			error = err instanceof Error ? err.message : 'revoke failed';
		}
	}

	onMount(load);
</script>

<div class="card bg-base-100 shadow">
	<div class="card-body">
		<h2 class="card-title">API keys</h2>
		{#if error}<div class="alert alert-error text-sm">{error}</div>{/if}
		{#if projects.length === 0}
			<p class="text-sm opacity-70">No projects yet — create one first.</p>
		{:else}
			<select class="select select-bordered w-full max-w-xs" bind:value={projectId} onchange={loadKeys}>
				{#each projects as p}<option value={p.id}>{p.name}</option>{/each}
			</select>
		{/if}
		{#if newSecret}
			<div class="alert alert-warning text-sm">
				<span>Copy the secret now — it is never shown again: <code class="font-mono">{newSecret}</code></span>
			</div>
		{/if}
		<table class="table table-zebra mt-2">
			<thead><tr><th>Name</th><th>Public key</th><th>Status</th><th></th></tr></thead>
			<tbody>
				{#if keys.length === 0}
					<tr><td colspan="4" class="text-center opacity-60">No keys yet.</td></tr>
				{:else}
					{#each keys as k}
						<tr>
							<td>{k.name}</td>
							<td class="font-mono text-xs">{k.publicKey}</td>
							<td>{k.revokedAt ? 'revoked' : 'active'}</td>
							<td>{#if !k.revokedAt}<button class="btn btn-xs" onclick={() => revoke(k.id)}>Revoke</button>{/if}</td>
						</tr>
					{/each}
				{/if}
			</tbody>
		</table>
		{#if projectId}
			<form class="flex gap-2 mt-2" onsubmit={create}>
				<input class="input input-bordered flex-1" placeholder="Key name (e.g. prod)" bind:value={keyName} required />
				<button class="btn btn-primary">Create key</button>
			</form>
		{/if}
	</div>
</div>
