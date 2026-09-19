import { clsx, type ClassValue } from 'clsx';
import { twMerge } from 'tailwind-merge';
import type { DockerImage } from './proto/discopanel/v1/minecraft_pb';
import type { Timestamp } from '@bufbuild/protobuf/wkt';

export function cn(...inputs: ClassValue[]) {
	return twMerge(clsx(inputs));
}

// eslint-disable-next-line @typescript-eslint/no-explicit-any
export type WithoutChild<T> = T extends { child?: any } ? Omit<T, 'child'> : T;
// eslint-disable-next-line @typescript-eslint/no-explicit-any
export type WithoutChildren<T> = T extends { children?: any } ? Omit<T, 'children'> : T;
export type WithoutChildrenOrChild<T> = WithoutChildren<WithoutChild<T>>;
export type WithElementRef<T, U extends HTMLElement = HTMLElement> = T & { ref?: U | null };

export function formatBytes(bytes: number, decimals = 2): string {
	if (bytes === 0) return '0 Bytes';

	const k = 1024;
	const dm = decimals < 0 ? 0 : decimals;
	const sizes = ['Bytes', 'KB', 'MB', 'GB', 'TB', 'PB', 'EB', 'ZB', 'YB'];

	const i = Math.floor(Math.log(bytes) / Math.log(k));

	return parseFloat((bytes / Math.pow(k, i)).toFixed(dm)) + ' ' + sizes[i];
}

export function getUniqueDockerImages(images: DockerImage[]): DockerImage[] {
	const seen = new Map<string, DockerImage>();
	for (const image of images) {
		if (!seen.has(image.tag)) {
			seen.set(image.tag, image);
		}
	}
	return Array.from(seen.values());
}

export function getDockerImageDisplayName(
	tagOrImage: string | DockerImage,
	dockerImages?: DockerImage[]
): string {
	const image =
		typeof tagOrImage === 'string'
			? dockerImages?.find((img) => img.tag === tagOrImage)
			: tagOrImage;
	if (!image) return tagOrImage as string;
	// Use the displayName field from the generated type
	return image.displayName || image.tag;
}

export function getStringForEnum(map: Record<string, unknown>, val: unknown) {
	return Object.keys(map).find((key) => map[key] === val);
}

// Convert protobuf Timestamp (bigint seconds) to a JS Date
export function timestampToDate(timestamp: Timestamp | undefined | null): Date {
	if (!timestamp) return new Date(0);
	return new Date(Number(timestamp.seconds) * 1000 + timestamp.nanos / 1_000_000);
}

// Human-readable duration, e.g. "3h 24m", "45m", "2d 5h"
export function formatPlaytime(secs: number | bigint): string {
	const total = Number(secs);
	if (!total || total <= 0) return '—';
	const days = Math.floor(total / 86400);
	const hours = Math.floor((total % 86400) / 3600);
	const minutes = Math.floor((total % 3600) / 60);
	if (days > 0) return `${days}d ${hours}h`;
	if (hours > 0) return `${hours}h ${minutes}m`;
	if (minutes > 0) return `${minutes}m`;
	return `${total}s`;
}

// Human-readable elapsed time since a date, e.g. "5m ago"
export function formatTimeAgo(date: Date, now: Date = new Date()): string {
	const diff = Math.max(0, Math.floor((now.getTime() - date.getTime()) / 1000));
	if (date.getTime() === 0) return 'Never';
	const days = Math.floor(diff / 86400);
	const hours = Math.floor((diff % 86400) / 3600);
	const minutes = Math.floor((diff % 3600) / 60);
	if (days > 0) return `${days}d ago`;
	if (hours > 0) return `${hours}h ago`;
	if (minutes > 0) return `${minutes}m ago`;
	return 'Just now';
}

// Convert proto enum value to the lowercase string name used by backend
// NOTE: ModLoader.MOD_LOADER_VANILLA (1) -> "vanilla"
export function enumToString(map: Record<string, unknown>, val: unknown): string {
	const enumKey = getStringForEnum(map, val);
	if (!enumKey) return '';
	const parts = enumKey.split('_');
	if (parts.length > 2) {
		return parts.slice(2).join('_').toLowerCase();
	}
	return enumKey.toLowerCase();
}
