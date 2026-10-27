// kitdemo's settings page: the kit's frame (service, log, about) and the Hello tab.
import 'occulite-ui/app.css';
import {defineTabs, mountFrame} from 'ccu-addon-kit/frame';

import strings from './i18n';
import Hello from './tabs/Hello.svelte';

const tabs = defineTabs([
    {id: 'hello', label: {de: 'Begrüßung', en: 'Greeting'}, component: Hello},
]);

void mountFrame(document.getElementById('app')!, {tabs, strings});
