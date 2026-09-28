<script lang="ts">
	import Router from 'svelte-spa-router';
	import { setToken, token } from './lib/api';
	import Dashboard from './pages/Dashboard.svelte';
	import Keys from './pages/Keys.svelte';
	import Login from './pages/Login.svelte';
	import Projects from './pages/Projects.svelte';
	import Prompts from './pages/Prompts.svelte';
	import Traces from './pages/Traces.svelte';

	const routes = {
		'/': Dashboard,
		'/login': Login,
		'/projects': Projects,
		'/keys': Keys,
		'/traces': Traces,
		'/prompts': Prompts
	};

	function logout() {
		setToken(null);
		location.hash = '#/login';
	}
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
			{#if $token}
				<li><a href="#/projects">Projects</a></li>
				<li><a href="#/keys">Keys</a></li>
				<li><button onclick={logout}>Logout</button></li>
			{:else}
				<li><a href="#/login">Login</a></li>
			{/if}
		</ul>
	</div>
</div>

<main class="container mx-auto max-w-5xl p-4">
	<Router {routes} />
</main>
