import { Injectable } from '@angular/core';

import { AdmissionMonitorConfig } from '../interfaces';

@Injectable({
    providedIn: 'root'
})
export class AdmissionHelperService {
    /**
     * convertConfigToDiffValues normalizes a config loaded from the API so that it can be compared
     * with the one built from the settings form. Unlike runtime-monitor's config, admission-monitor's
     * one has no fields the form does not own, so only the key order is normalized.
     */
    static convertConfigToDiffValues(config: AdmissionMonitorConfig): AdmissionMonitorConfig {
        return {
            version: config.version,
            policies: config.policies,
            history_control: config.history_control
        };
    }
}
