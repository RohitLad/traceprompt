<script lang="ts">
	import { onMount } from 'svelte';
	import { get } from 'svelte/store';
	import {
		API_BASE,
		createPrompt,
		createPromptVersion,
		currentProjectId,
		getPromptDetail,
		listProjects,
		listPrompts,
		setProject,
		setPromptLabels,
		token,
		type Project,
		type PromptDetail,
		type PromptSummary
	} from '../lib/api';

	let projects = $state<Project[]>([]);
	let projectId = $state('');
	let prompts = $state<PromptSummary[]>([]);
	let selected = $state<PromptDetail | null>(null);
	let error = $state('');

	// create form
	let newName = $state('');
	let newType = $state('text');
	let newTemplate = $state('');

	// version form
	let versionTemplate = $state('');
	let versionMsg = $state('');

	async function load(projectsFirst = true) {
		error = '';
		try {
			if (projectsFirst || projects.length === 0) {
				projects = await listProjects(API_BASE, get(token) ?? '');
				projectId = get(currentProjectId) ?? projects[0]?.id ?? '';
				setProject(projectId);
			}
			if (!projectId) {
				error = 'No projects yet — create one first.';
				return;
			}
			prompts = await listPrompts(API_BASE, get(token) ?? '', projectId);
			if (selected) selected = await getPromptDetail(API_BASE, get(token) ?? '', projectId, selected.name);
		} catch (err) {
			error = err instanceof Error ? err.message : 'failed to load';
		}
	}

	async function create(e: Event) {
		e.preventDefault();
		error = '';
		try {
			await createPrompt(API_BASE, get(token) ?? '', projectId, {
				name: newName,
				type: newType,
				template: newType === 'text' ? newTemplate : undefined
			});
			newName = '';
			newTemplate = '';
			await load(false);
		} catch (err) {
			error = err instanceof Error ? err.message : 'create failed';
		}
	}

	async function addVersion(e: Event) {
		e.preventDefault();
		if (!selected) return;
		try {
			await createPromptVersion(API_BASE, get(token) ?? '', projectId, selected.name, {
				template: versionTemplate,
				commitMessage: versionMsg || undefined
			});
			versionTemplate = '';
			versionMsg = '';
			await load(false);
		} catch (err) {
			error = err instanceof Error ? err.message : 'version failed';
		}
	}

	async function promote(version: number) {
		if (!selected) return;
		try {
			await setPromptLabels(API_BASE, get(token) ?? '', projectId, selected.name, version, ['production']);
			await load(false);
		} catch (err) {
			error = err instanceof Error ? err.message : 'promote failed';
		}
	}

	onMount(() => load());
</script>

<div class="grid md:grid-cols-3 gap-4">
	<div class="card bg-base-100 shadow">
		<div class="card-body">
			<div class="flex items-center justify-between gap-2">
				<h2 class="card-title">Prompts</h2>
				<select class="select select-bordered select-sm max-w-[10rem]" bind:value={projectId} onchange={() => { selected = null; load(false); }}>
					{#each projects as p}<option value={p.id}>{p.name}</option>{/each}
				</select>
			</div>
			{#if error}<div class="alert alert-error text-sm">{error}</div>{/if}
			<ul class="menu bg-base-200 rounded-box">
				{#each prompts as p}
					<li>
						<button
							class:active={selected?.name === p.name}
							onclick={async () => {
								selected = await getPromptDetail(API_BASE, get(token) ?? '', projectId, p.name);
							}}
						>
							{p.name}
							<span class="badge badge-sm">v{p.versions}</span>
							{#if p.productionVersion}<span class="badge badge-success badge-sm">prod v{p.productionVersion}</span>{/if}
						</button>
					</li>
				{/each}
			</ul>
			{#if prompts.length === 0}<p class="text-sm opacity-60">No prompts yet.</p>{/if}
			<form class="space-y-2 mt-2" onsubmit={create}>
				<input class="input input-bordered w-full input-sm" placeholder="New prompt name" bind:value={newName} required />
				<textarea class="textarea textarea-bordered w-full text-sm" placeholder="Template with variables like name" bind:value={newTemplate} required></textarea>
				<button class="btn btn-primary btn-sm w-full">Create prompt</button>
			</form>
		</div>
	</div>

	<div class="card bg-base-100 shadow md:col-span-2">
		<div class="card-body">
			{#if !selected}
				<p class="opacity-60 text-sm">Select a prompt to see versions, or create one.</p>
			{:else}
				<h2 class="card-title">{selected.name} <span class="badge">{selected.type}</span></h2>
				{#each [...selected.versions].reverse() as v}
					<div class="border rounded-box p-3 mb-2">
						<div class="flex items-center gap-2 flex-wrap">
							<span class="badge">v{v.version}</span>
							{#each v.labels as l}<span class="badge badge-outline badge-sm">{l}</span>{/each}
							{#if !v.labels.includes('production')}
								<button class="btn btn-xs" onclick={() => promote(v.version)}>Promote to production</button>
							{/if}
						</div>
						{#if v.commitMessage}<p class="text-xs opacity-60 mt-1">{v.commitMessage}</p>{/if}
						<pre class="text-xs bg-base-200 rounded p-2 mt-2 overflow-x-auto">{v.template ?? JSON.stringify(v.messages, null, 1)}</pre>
					</div>
				{/each}
				<form class="space-y-2 mt-2" onsubmit={addVersion}>
					<textarea class="textarea textarea-bordered w-full text-sm" placeholder="New version template" bind:value={versionTemplate} required></textarea>
					<input class="input input-bordered w-full input-sm" placeholder="Commit message (optional)" bind:value={versionMsg} />
					<button class="btn btn-secondary btn-sm">Add version</button>
				</form>
			{/if}
		</div>
	</div>
</div>
