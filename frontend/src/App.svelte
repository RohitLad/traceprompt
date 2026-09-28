<script lang="ts">
	import Router from 'svelte-spa-router';
	import Dashboard from './pages/Dashboard.svelte';
	import Traces from './pages/Traces.svelte';
	import Prompts from './pages/Prompts.svelte';

	const routes = {
		'/': Dashboard,
		'/traces': Traces,
		'/prompts': Prompts
	};
</script>

<div class="navbar bg-base-100 shadow" data-testid="navbar">
	<div class="flex-1">
		<a class="btn btn-ghost text-xl" href="#/">🪢 traceprompt</a>
	</div>
	<div class="flex-none">
		<ul class="menu menu-horizontal px-1">
			<li><a href="#/">Dashboard</a></li>
			<li><a href="#/traces">Traces</a></li>
			<li><a href="#/prompts">Prompts</a></li>
		</ul>
	</div>
</div>

<main class="container mx-auto max-w-5xl p-4">
	<Router {routes} />
</main>
