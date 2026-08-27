import { Injectable } from '@angular/core';

import { AdmissionMonitorConfig, AdmissionMonitorPolicies } from '@cs/domains/admission';

import { AdmissionSettingForm } from '../interfaces/admission-form.interface';

@Injectable({
    providedIn: 'root'
})
export class AdmissionFeatureConfigUtilsService {
    static convertSettingFormToMonitorConfig(formValues: AdmissionSettingForm): AdmissionMonitorConfig {
        const policies = Object.entries(formValues.policies).reduce((acc, [key, value]) => {
            acc[key] = {
                name: value.name,
                enabled: value.isEnabled,
                action: value.action,
                severity: value.severity,
                description: value.description || undefined,
                yaml: value.yaml || undefined
            };

            return acc;
        }, {} as AdmissionMonitorPolicies);

        return {
            version: '1', // @todo: create environment constant
            policies,
            history_control: formValues.historyControl
        };
    }
}
