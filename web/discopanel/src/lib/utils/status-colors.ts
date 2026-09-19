import { ServerStatus } from '$lib/proto/discopanel/v1/common_pb';

/**
 * Shared status color mapping so every page renders the same concept with the
 * same colors: running/online = green, transitioning = yellow, stopped = gray,
 * error/unhealthy = red.
 */

/** Tinted-outline badge classes for a server status (use with variant="outline"). */
export function serverStatusBadgeClass(status: ServerStatus): string {
	switch (status) {
		case ServerStatus.RUNNING:
			return 'border-green-500/20 bg-green-500/10 text-green-500 dark:text-green-400';
		case ServerStatus.STARTING:
		case ServerStatus.STOPPING:
		case ServerStatus.CREATING:
		case ServerStatus.RESTARTING:
			return 'border-yellow-500/20 bg-yellow-500/10 text-yellow-500 dark:text-yellow-400';
		case ServerStatus.ERROR:
		case ServerStatus.UNHEALTHY:
			return 'border-red-500/20 bg-red-500/10 text-red-500 dark:text-red-400';
		case ServerStatus.STOPPED:
		default:
			return 'border-gray-500/20 bg-gray-500/10 text-gray-500 dark:text-gray-400';
	}
}

/** Small status dot classes for a server status. */
export function serverStatusDotClass(status: ServerStatus): string {
	switch (status) {
		case ServerStatus.RUNNING:
			return 'bg-green-500 animate-pulse';
		case ServerStatus.STARTING:
		case ServerStatus.STOPPING:
		case ServerStatus.CREATING:
		case ServerStatus.RESTARTING:
			return 'bg-yellow-500 animate-pulse';
		case ServerStatus.ERROR:
		case ServerStatus.UNHEALTHY:
			return 'bg-red-500 animate-pulse';
		case ServerStatus.STOPPED:
		default:
			return 'bg-gray-400';
	}
}

/** Human-readable, title-cased label for a server status. */
export function serverStatusLabel(status: ServerStatus): string {
	switch (status) {
		case ServerStatus.RUNNING:
			return 'Running';
		case ServerStatus.STOPPED:
			return 'Stopped';
		case ServerStatus.STARTING:
			return 'Starting';
		case ServerStatus.STOPPING:
			return 'Stopping';
		case ServerStatus.ERROR:
			return 'Error';
		case ServerStatus.CREATING:
			return 'Creating';
		case ServerStatus.RESTARTING:
			return 'Restarting';
		case ServerStatus.UNHEALTHY:
			return 'Unhealthy';
		default:
			return 'Unknown';
	}
}

/** Tinted-outline badge classes for "online / active / ok" states. */
export const ONLINE_BADGE_CLASS =
	'border-green-500/20 bg-green-500/10 text-green-500 dark:text-green-400';

/** Tinted-outline badge classes for "error" states. */
export const ERROR_BADGE_CLASS =
	'border-red-500/20 bg-red-500/10 text-red-500 dark:text-red-400';
