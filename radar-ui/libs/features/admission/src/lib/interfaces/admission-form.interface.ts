import {
    AdmissionMonitorHistoryControl,
    AdmissionMonitorPolicyAction,
    AdmissionMonitorSeverity
} from '@cs/domains/admission';

export interface AdmissionSettingPolicyForm {
    isEnabled: boolean;
    name: string;
    description: string;
    yaml: string;
    action: AdmissionMonitorPolicyAction;
    severity: AdmissionMonitorSeverity;
}

export type AdmissionSettingPolicyRecord = {
    [key: string]: AdmissionSettingPolicyForm;
};

export interface AdmissionSettingForm {
    policies: AdmissionSettingPolicyRecord;
    historyControl: AdmissionMonitorHistoryControl;
}

export interface AdmissionExpertModeForm {
    isExpert: boolean;
}
