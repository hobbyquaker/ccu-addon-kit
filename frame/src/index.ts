// ccu-addon-kit/frame (MIT): the settings frame of a classic CCU addon, on occulite-ui.
// The contract is docs/tab-contract.md.

export {mountFrame, defineTabs, toCatalogue, type MountOptions, type AddonStrings} from './mountFrame.js';
export {getKit} from './context.js';
export {KitRequestError, isKitError, REQUEST_HEADER, type ApiRaw} from './api.js';
export {statusView, formatKb, formatDuration} from './status.js';
export {FRAME_MESSAGES} from './messages.js';
export {default as KitLoading} from './components/KitLoading.svelte';
export {default as KitError} from './components/KitError.svelte';
export {default as KitEmpty} from './components/KitEmpty.svelte';
export type * from './types.js';
