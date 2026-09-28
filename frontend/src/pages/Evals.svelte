<script lang="ts">
	import { onMount } from 'svelte';
	import { get } from 'svelte/store';
	import {
		API_BASE,
		addQueueItem,
		completeQueueItem,
		createQueue,
		createScoreConfig,
		currentProjectId,
		getQueue,
		listProjects,
		listQueues,
		listScoreConfigs,
		scoreQueueItem,
		setProject,
		token,
		type Project,
		type QueueItem,
		type ReviewQueue,
		type ScoreConfig
	} from '../lib/api';

	let projects = $state<Project[]>([]);
	let projectId = $state('');
	let configs = $state<ScoreConfig[]>([]);
	let queues = $state<ReviewQueue[]>([]);
	let selectedQueue = $state<string>('');
	let items = $state<QueueItem[]>([]);
	let showCompleted = $state(false);
	let error = $state('');

	// config form
	let cfgName = $state('');
	let cfgType = $state('NUMERIC');
	let cfgMin = $state('0');
	let cfgMax = $state('1');
	let cfgCats = $state('good, bad');

	// queue forms
	let queueName = $state('');
	let newTraceId = $state('');
	let scoreName = $state<Record<string, string>>({});
	let scoreValue = $state<Record<string, string>>({});

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
			const t = get(token) ?? '';
			configs = await listScoreConfigs(API_BASE, t, projectId);
			queues = await listQueues(API_BASE, t, projectId);
			if (selectedQueue) await loadItems();
		} catch (err) {
			error = err instanceof Error ? err.message : 'failed to load';
		}
	}

	async function loadItems() {
		items = await getQueue(API_BASE, get(token) ?? '', projectId, selectedQueue, showCompleted ? undefined : 'PENDING');
	}

	async function createCfg(e: Event) {
		e.preventDefault();
		try {
			await createScoreConfig(API_BASE, get(token) ?? '', projectId, {
				name: cfgName,
				dataType: cfgType,
				minValue: cfgType === 'NUMERIC' ? Number(cfgMin) : undefined,
				maxValue: cfgType === 'NUMERIC' ? Number(cfgMax) : undefined,
				categories: cfgType === 'CATEGORICAL' ? cfgCats.split(',').map((s) => s.trim()).filter(Boolean) : undefined
			});
			cfgName = '';
			await load();
		} catch (err) {
			error = err instanceof Error ? err.message : 'create failed';
		}
	}

	async function createQ(e: Event) {
		e.preventDefault();
		try {
			await createQueue(API_BASE, get(token) ?? '', projectId, queueName);
			queueName = '';
			await load();
		} catch (err) {
			error = err instanceof Error ? err.message : 'create failed';
		}
	}

	async function addItem(e: Event) {
		e.preventDefault();
		try {
			await addQueueItem(API_BASE, get(token) ?? '', projectId, selectedQueue, newTraceId);
			newTraceId = '';
			await load();
		} catch (err) {
			error = err instanceof Error ? err.message : 'add failed';
		}
	}

	async function submitScore(itemId: string) {
		try {
			await scoreQueueItem(API_BASE, get(token) ?? '', projectId, selectedQueue, itemId, scoreName[itemId] ?? '', scoreValue[itemId] ?? '');
			scoreName[itemId] = '';
			scoreValue[itemId] = '';
			await load();
		} catch (err) {
			error = err instanceof Error ? err.message : 'score failed (check the score schema)';
		}
	}

	async function complete(itemId: string) {
		try {
			await completeQueueItem(API_BASE, get(token) ?? '', projectId, selectedQueue, itemId);
			await load();
		} catch (err) {
			error = err instanceof Error ? err.message : 'complete failed';
		}
	}

	onMount(load);
</script>

<div class="grid md:grid-cols-2 gap-4">
	<div class="card bg-base-100 shadow">
		<div class="card-body">
			<div class="flex items-center justify-between gap-2">
				<h2 class="card-title">Score schemas</h2>
				<select class="select select-bordered select-sm max-w-[10rem]" bind:value={projectId} onchange={load}>
					{#each projects as p}<option value={p.id}>{p.name}</option>{/each}
				</select>
			</div>
			{#if error}<div class="alert alert-error text-sm">{error}</div>{/if}
			<p class="text-xs opacity-60">Scores matching a schema are validated on ingest; violations are rejected, never stored silently.</p>
			<ul class="space-y-1">
				{#each configs as c}
					<li class="border rounded p-2 text-sm">
						<span class="font-medium">{c.name}</span>
						<span class="badge badge-sm ml-1">{c.dataType}</span>
						{#if c.dataType === 'NUMERIC'}<span class="text-xs opacity-60 ml-1">{c.minValue}–{c.maxValue}</span>{/if}
						{#if c.dataType === 'CATEGORICAL'}<span class="text-xs opacity-60 ml-1">{c.categories.join(', ')}</span>{/if}
					</li>
				{/each}
			</ul>
			{#if configs.length === 0}<p class="text-sm opacity-60">No schemas — all score names accepted as-is.</p>{/if}
			<form class="space-y-2 mt-2" onsubmit={createCfg}>
				<input class="input input-bordered w-full input-sm" placeholder="Schema name (e.g. quality)" bind:value={cfgName} required />
				<select class="select select-bordered select-sm w-full" bind:value={cfgType}>
					<option>NUMERIC</option><option>CATEGORICAL</option><option>BOOLEAN</option>
				</select>
				{#if cfgType === 'NUMERIC'}
					<div class="flex gap-2">
						<input class="input input-bordered input-sm flex-1" type="number" step="any" bind:value={cfgMin} />
						<input class="input input-bordered input-sm flex-1" type="number" step="any" bind:value={cfgMax} />
					</div>
				{:else if cfgType === 'CATEGORICAL'}
					<input class="input input-bordered w-full input-sm" placeholder="comma-separated categories" bind:value={cfgCats} />
				{/if}
				<button class="btn btn-primary btn-sm w-full">Create schema</button>
			</form>
		</div>
	</div>

	<div class="card bg-base-100 shadow">
		<div class="card-body">
			<h2 class="card-title">Review queues</h2>
			<ul class="menu bg-base-200 rounded-box">
				{#each queues as q}
					<li>
						<button class:active={selectedQueue === q.id} onclick={() => { selectedQueue = q.id; loadItems(); }}>
							{q.name}
							<span class="badge badge-sm">{q.pending} pending</span>
							<span class="badge badge-sm badge-ghost">{q.total} total</span>
						</button>
					</li>
				{/each}
			</ul>
			<form class="flex gap-2 mt-2" onsubmit={createQ}>
				<input class="input input-bordered flex-1 input-sm" placeholder="New queue name" bind:value={queueName} required />
				<button class="btn btn-secondary btn-sm">Create</button>
			</form>
			{#if selectedQueue}
				<div class="divider">Items</div>
				<label class="label cursor-pointer justify-start gap-2">
					<input type="checkbox" class="checkbox checkbox-sm" bind:checked={showCompleted} onchange={loadItems} />
					<span class="label-text text-xs">Show completed</span>
				</label>
				<ul class="space-y-2">
					{#each items as it}
						<li class="border rounded p-2 text-sm">
							<div class="flex items-center justify-between gap-2">
								{#if it.traceId}<a class="link font-mono text-xs" href="#/traces/{it.traceId}">{it.traceName ?? it.traceId}</a>
								{:else}<span class="opacity-60">no trace</span>{/if}
								{#if it.status === 'PENDING'}
									<button class="btn btn-xs" onclick={() => complete(it.id)}>Complete</button>
								{:else}
									<span class="badge badge-success badge-sm">done</span>
								{/if}
							</div>
							{#if it.status === 'PENDING'}
								<form class="flex gap-1 mt-1" onsubmit={(e) => { e.preventDefault(); submitScore(it.id); }}>
									<input class="input input-bordered input-xs flex-1" placeholder="score name" bind:value={scoreName[it.id]} required />
									<input class="input input-bordered input-xs flex-1" placeholder="value" bind:value={scoreValue[it.id]} required />
									<button class="btn btn-xs">Score</button>
								</form>
							{/if}
						</li>
					{/each}
				</ul>
				{#if items.length === 0}<p class="text-sm opacity-60">Queue is clear.</p>{/if}
				<form class="flex gap-2 mt-2" onsubmit={addItem}>
					<input class="input input-bordered flex-1 input-sm font-mono" placeholder="trace id to review" bind:value={newTraceId} required />
					<button class="btn btn-sm">Add</button>
				</form>
			{/if}
		</div>
	</div>
</div>
