// The error boundary and the lazy loading of a tab (TabHost), mounted in jsdom.
import {describe, expect, it, vi} from 'vitest';
import {flushSync, mount, tick, unmount} from 'svelte';

import TabHost from '../src/components/TabHost.svelte';
import Fine from './Fine.svelte';
import Throws from './Throws.svelte';

describe('TabHost', () => {
    it('catches a tab that throws and offers Retry', async () => {
        const err = vi.spyOn(console, 'error').mockImplementation(() => {});
        const el = document.createElement('div');
        const app = mount(TabHost, {target: el, props: {component: Throws, active: true, path: ''}});
        flushSync();
        expect(el.querySelector('[data-testid="kit-boundary"]')).not.toBeNull();
        expect(el.textContent).toContain('the tab broke');
        await unmount(app);
        err.mockRestore();
    });

    it('loads a lazy tab when it is first shown', async () => {
        const loader = vi.fn(async () => ({default: Fine}));
        const el = document.createElement('div');
        const props = $state({component: loader, active: false, path: 'x'});
        const app = mount(TabHost, {target: el, props});
        flushSync();
        expect(loader).not.toHaveBeenCalled();
        props.active = true;
        flushSync();
        await vi.waitFor(() => expect(el.querySelector('[data-testid="fine"]')?.textContent).toBe('fine x'));
        expect(loader).toHaveBeenCalledOnce();
        await tick();
        await unmount(app);
    });

    it('a lazy tab that fails to load shows the failure', async () => {
        const err = vi.spyOn(console, 'error').mockImplementation(() => {});
        const el = document.createElement('div');
        const app = mount(TabHost, {target: el, props: {component: async () => { throw new Error('chunk missing'); }, active: true, path: ''}});
        await vi.waitFor(() => expect(el.textContent).toContain('chunk missing'));
        await unmount(app);
        err.mockRestore();
    });
});
