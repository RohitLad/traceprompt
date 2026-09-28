<script lang="ts">
	import { onMount } from 'svelte';
	import { get } from 'svelte/store';
	import {
		API_BASE,
		currentProjectId,
		listProjects,
		listSessions,
		setProject,
		token,
		type Project,
		type Session
	} from '../lib/api';

	let projects = $state<Project[]>([]);
	let projectId = $state('');
	let sessions = $state<Session[]>([]);
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
			sessions = await listSessions(API_BASE, get(token) ?? '', projectId);
		} catch (err) {
			error = err instanceof Error ? err.message : 'failed to load';
		}
	}

	onMount(load);
</script>

<div class="card bg-base-100 shadow">
	<div class="card-body">
		<div class="flex items-center justify-between flex-wrap gap-2">
			<h2 class="card-title">Sessions</h2>
			<select class="select select-bordered select-sm max-w-[10rem]" bind:value={projectId} onchange={load}>
				{#each projects as p}<option value={p.id}>{p.name}</option>{/each}
			</select>
		</div>
		{#if error}<div class="alert alert-error text-sm">{error}</div>{/if}
		<ul class="menu bg-base-200 rounded-box">
			{#each sessions as s}
				<li>
					<a href="#/traces" title="Open traces">
						<span class="font-mono text-xs">{s.sessionId}</span>
						<span class="badge badge-sm">{s.traceCount} traces</span>
						<span class="text-xs opacity-60">{new Date(s.lastTraceAt).toLocaleString()}</span>
					</a>
				</li>
			{/each}
		</ul>
		{#if sessions.length === 0}<p class="text-sm opacity-60">No sessions yet — set sessionId on your traces to group conversations.</p>{/if}
	</div>
</div>
