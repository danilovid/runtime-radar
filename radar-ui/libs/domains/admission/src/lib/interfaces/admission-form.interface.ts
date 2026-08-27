import {
    AdmissionMonitorHistoryControl,
    AdmissionMonitorPolicyAction,
    AdmissionMonitorSeverity
} from './contract/admission-monitor-contract.interface';

export interface AdmissionPolicyActionOption {
    id: AdmissionMonitorPolicyAction;
    localizationKey: string;
}

export interface AdmissionSeverityOption {
    id: AdmissionMonitorSeverity;
    localizationKey: string;
}

export interface AdmissionHistoryControlOption {
    id: AdmissionMonitorHistoryControl;
    localizationKey: string;
}
