<script lang="ts">
	import { onMount } from 'svelte';
	import { get } from 'svelte/store';
	import { API_BASE, authHeaders, currentProjectId, listProjects, token, type Project } from '../lib/api';

	interface Key {
		id: string;
		name: string;
		publicKey: string;
		createdAt: string;
		revokedAt: string | null;
	}

	let projects = $state<Project[]>([]);
	let projectId = $state('');
	let keys = $state<Key[]>([]);
	let keyName = $state('');
	let newSecret = $state('');
	let error = $state('');

	async function load() {
		try {
			projects = await listProjects(API_BASE, get(token) ?? '');
			projectId = get(currentProjectId) ?? projects[0]?.id ?? '';
			if (projectId) await loadKeys();
		} catch (err) {
			error = err instanceof Error ? err.message : 'failed to load';
		}
	}

	async function loadKeys() {
		const res = await fetch(`${API_BASE}/api/v1/projects/${projectId}/keys`, {
			headers: authHeaders(get(token))
		});
		if (!res.ok) throw new Error('failed to load keys');
		keys = (((await res.json()) as { data: Key[] }).data ?? []);
	}

	async function create(e: Event) {
		e.preventDefault();
		newSecret = '';
		const res = await fetch(`${API_BASE}/api/v1/projects/${projectId}/keys`, {
			method: 'POST',
			headers: { 'Content-Type': 'application/json', ...authHeaders(get(token)) },
			body: JSON.stringify({ name: keyName })
		});
		if (!res.ok) {
			error = 'create failed';
			return;
		}
		const created = (await res.json()) as Key & { secret: string };
		newSecret = created.secret;
		keyName = '';
		await loadKeys();
	}

	async function revoke(id: string) {
		await fetch(`${API_BASE}/api/v1/projects/${projectId}/keys/${id}/revoke`, {
			method: 'POST',
			headers: authHeaders(get(token))
		});
		await loadKeys();
	}

	onMount(load);
</script>

<div class="card bg-base-100 shadow">
	<div class="card-body">
		<h2 class="card-title">API keys</h2>
		{#if error}<div class="alert alert-error text-sm">{error}</div>{/if}
		<select class="select select-bordered w-full max-w-xs" bind:value={projectId} onchange={loadKeys}>
			{#each projects as p}<option value={p.id}>{p.name}</option>{/each}
		</select>
		{#if newSecret}
			<div class="alert alert-warning text-sm">
				<span>Copy the secret now — it is never shown again: <code class="font-mono">{newSecret}</code></span>
			</div>
		{/if}
		<table class="table table-zebra mt-2">
			<thead><tr><th>Name</th><th>Public key</th><th>Status</th><th></th></tr></thead>
			<tbody>
				{#each keys as k}
					<tr>
						<td>{k.name}</td>
						<td class="font-mono text-xs">{k.publicKey}</td>
						<td>{k.revokedAt ? 'revoked' : 'active'}</td>
						<td>{#if !k.revokedAt}<button class="btn btn-xs" onclick={() => revoke(k.id)}>Revoke</button>{/if}</td>
					</tr>
				{/each}
			</tbody>
		</table>
		<form class="flex gap-2 mt-2" onsubmit={create}>
			<input class="input input-bordered flex-1" placeholder="Key name (e.g. prod)" bind:value={keyName} required />
			<button class="btn btn-primary">Create key</button>
		</form>
	</div>
</div>
