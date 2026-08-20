import { InjectionToken } from '@angular/core';

/**
 * Where "Report a problem" sends the request the assistant helped write. It is
 * deployment-specific, so it comes from the environment rather than a constant:
 * an installation supported by its own team does not write to the vendor.
 */
export const SUPPORT_EMAIL = new InjectionToken<string>('supportEmail');
