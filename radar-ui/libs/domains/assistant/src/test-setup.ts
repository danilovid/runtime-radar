/* eslint import/no-unassigned-import: "off" */
import { setupZoneTestEnv } from 'jest-preset-angular/setup-env/zone';

// jest-preset-angular 14 deprecated the "setup-jest" entry point, which the
// installed zone.js only ships as ESM and jest therefore cannot load.
setupZoneTestEnv();

// The jsdom build in use predates Blob.text(), which every browser this product
// supports has and which is how attachments are read. Without this, a test of
// that code would be testing the absence of a method rather than the budget.
if (!Blob.prototype.text) {
    Blob.prototype.text = function text(this: Blob): Promise<string> {
        return new Promise((resolve, reject) => {
            const reader = new FileReader();

            reader.onload = () => resolve(String(reader.result));
            reader.onerror = () => reject(reader.error);
            reader.readAsText(this);
        });
    };
}
