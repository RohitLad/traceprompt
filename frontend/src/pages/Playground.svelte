<script lang="ts">
	import { onMount } from 'svelte';
	import { get } from 'svelte/store';
	import {
		API_BASE,
		currentProjectId,
		getPromptDetail,
		listProjects,
		listPrompts,
		runPlayground,
		setProject,
		token,
		type PlaygroundResult,
		type Project,
		type PromptSummary
	} from '../lib/api';

	let projects = $state<Project[]>([]);
	let projectId = $state('');
	let prompts = $state<PromptSummary[]>([]);
	let promptName = $state('');
	let template = $state('Hello {{name}}');
	let variablesText = $state('{"name": "Ada"}');
	let baseUrl = $state('https://api.openai.com/v1');
	let model = $state('gpt-4o-mini');
	let apiKey = $state(''); // component state only — never persisted
	let saveAsTrace = $state(true);
	let result = $state<PlaygroundResult | null>(null);
	let error = $state('');
	let running = $state(false);

	async function load() {
		try {
			projects = await listProjects(API_BASE, get(token) ?? '');
			projectId = get(currentProjectId) ?? projects[0]?.id ?? '';
			setProject(projectId);
			if (projectId) prompts = await listPrompts(API_BASE, get(token) ?? '', projectId);
		} catch (err) {
			error = err instanceof Error ? err.message : 'failed to load';
		}
	}

	async function usePrompt() {
		if (!promptName) return;
		try {
			const detail = await getPromptDetail(API_BASE, get(token) ?? '', projectId, promptName);
			const prod = detail.versions.find((v) => v.labels.includes('production')) ?? detail.versions[0];
			template = prod?.template ?? '';
		} catch (err) {
			error = err instanceof Error ? err.message : 'failed to load prompt';
		}
	}

	async function run(e: Event) {
		e.preventDefault();
		error = '';
		result = null;
		running = true;
		try {
			let variables: Record<string, string> = {};
			if (variablesText.trim()) variables = JSON.parse(variablesText) as Record<string, string>;
			result = await runPlayground(API_BASE, get(token) ?? '', projectId, {
				template,
				variables,
				provider: { baseUrl, apiKey, model },
				saveAsTrace
			});
		} catch (err) {
			error = err instanceof Error ? err.message : 'run failed';
		} finally {
			running = false;
		}
	}

	onMount(load);
</script>

<div class="grid md:grid-cols-2 gap-4">
	<div class="card bg-base-100 shadow">
		<div class="card-body">
			<div class="flex items-center justify-between gap-2 flex-wrap">
				<h2 class="card-title">Playground</h2>
				<select class="select select-bordered select-sm max-w-[10rem]" bind:value={projectId} onchange={load}>
					{#each projects as p}<option value={p.id}>{p.name}</option>{/each}
				</select>
			</div>
			{#if error}<div class="alert alert-error text-sm">{error}</div>{/if}
			<form class="space-y-2" onsubmit={run}>
				<div class="flex gap-2">
					<select class="select select-bordered select-sm flex-1" bind:value={promptName} onchange={usePrompt}>
						<option value="">Custom template…</option>
						{#each prompts as p}<option value={p.name}>{p.name}</option>{/each}
					</select>
				</div>
				<textarea class="textarea textarea-bordered w-full font-mono text-sm" rows="4" bind:value={template} required></textarea>
				<textarea class="textarea textarea-bordered w-full font-mono text-xs" rows="2" bind:value={variablesText}></textarea>
				<div class="grid grid-cols-2 gap-2">
					<input class="input input-bordered input-sm" bind:value={baseUrl} placeholder="https://api.openai.com/v1" />
					<input class="input input-bordered input-sm" bind:value={model} placeholder="model" />
				</div>
				<input class="input input-bordered w-full input-sm" type="password" bind:value={apiKey} placeholder="Provider API key (never stored)" required />
				<label class="label cursor-pointer justify-start gap-2">
					<input type="checkbox" class="checkbox checkbox-sm" bind:checked={saveAsTrace} />
					<span class="label-text">Save run as trace</span>
				</label>
				<button class="btn btn-primary w-full" disabled={running}>{running ? 'Running…' : 'Run'}</button>
			</form>
		</div>
	</div>
	<div class="card bg-base-100 shadow">
		<div class="card-body">
			<h2 class="card-title">Output</h2>
			{#if result}
				<pre class="bg-base-200 rounded p-3 text-sm whitespace-pre-wrap">{result.output}</pre>
				<div class="text-xs opacity-70 mt-2">
					model {result.model} · {result.inputTokens}/{result.outputTokens} tokens · {result.latencyMs}ms
					{#if result.traceId} · <a class="link font-mono" href="#/traces/{result.traceId}">{result.traceId}</a>{/if}
				</div>
			{:else}
				<p class="text-sm opacity-60">Run a prompt to see output, usage, and the saved trace link.</p>
			{/if}
		</div>
	</div>
</div>
