import type {PluginOption} from 'vite';
import type {Options as SvelteOptions} from '@sveltejs/vite-plugin-svelte';

export interface KitViteOptions {
    /** Where the build goes; `dist` next to the Vite config by default (vendor --frame copies it). */
    outDir?: string;
    /** `false` when the config adds the Svelte plugin itself; an object passes options to it. */
    svelte?: boolean | Partial<SvelteOptions>;
}

/** The `id` literals inside every `defineTabs(...)` call of a module's source. */
export function scanTabIds(code: string): string[];
export function kitVite(options?: KitViteOptions): PluginOption[];
export default kitVite;
