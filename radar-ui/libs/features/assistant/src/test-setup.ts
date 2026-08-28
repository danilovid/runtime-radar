/* eslint import/no-unassigned-import: "off" */
import { setupZoneTestEnv } from 'jest-preset-angular/setup-env/zone';

// jest-preset-angular 14 deprecated the "setup-jest" entry point, which the
// installed zone.js only ships as ESM and jest therefore cannot load.
setupZoneTestEnv();
