import {getContext, setContext} from 'svelte';

import type {Kit} from './types.js';

const KEY = Symbol('ccu-addon-kit');

/** @internal the frame sets it once, around every tab */
export function setKit(kit: Kit): void {
    setContext(KEY, kit);
}

/** The frame's context in a tab component (call it during the component's initialisation). */
export function getKit(): Kit {
    const kit = getContext<Kit | undefined>(KEY);
    if (!kit) {
        throw new Error('getKit(): not inside the ccu-addon-kit frame (call it in a tab component, during its setup)');
    }
    return kit;
}
