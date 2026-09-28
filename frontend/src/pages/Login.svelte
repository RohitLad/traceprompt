<script lang="ts">
	import { API_BASE, login, register, setToken } from '../lib/api';

	let mode = $state<'login' | 'register'>('login');
	let email = $state('');
	let password = $state('');
	let name = $state('');
	let orgName = $state('');
	let error = $state('');
	let busy = $state(false);

	async function submit(e: Event) {
		e.preventDefault();
		error = '';
		busy = true;
		try {
			const out =
				mode === 'login'
					? await login(API_BASE, email, password)
					: await register(API_BASE, email, password, name, orgName);
			setToken(out.token);
			location.hash = '#/projects';
		} catch (err) {
			error = err instanceof Error ? err.message : 'request failed';
		} finally {
			busy = false;
		}
	}
</script>

<div class="card bg-base-100 shadow max-w-md mx-auto">
	<div class="card-body">
		<h2 class="card-title">{mode === 'login' ? 'Sign in' : 'Create account'}</h2>
		{#if error}
			<div class="alert alert-error text-sm" role="alert">{error}</div>
		{/if}
		<form class="space-y-3" onsubmit={submit}>
			{#if mode === 'register'}
				<input class="input input-bordered w-full" placeholder="Name" bind:value={name} required />
				<input class="input input-bordered w-full" placeholder="Organization (optional)" bind:value={orgName} />
			{/if}
			<input class="input input-bordered w-full" type="email" placeholder="Email" bind:value={email} required />
			<input
				class="input input-bordered w-full"
				type="password"
				placeholder="Password (8+ chars)"
				bind:value={password}
				minlength={8}
				required
			/>
			<button class="btn btn-primary w-full" type="submit" disabled={busy}>
				{busy ? '…' : mode === 'login' ? 'Sign in' : 'Create account'}
			</button>
		</form>
		<button
			class="btn btn-ghost btn-sm"
			onclick={() => (mode = mode === 'login' ? 'register' : 'login')}
		>
			{mode === 'login' ? 'Need an account? Register' : 'Have an account? Sign in'}
		</button>
	</div>
</div>
