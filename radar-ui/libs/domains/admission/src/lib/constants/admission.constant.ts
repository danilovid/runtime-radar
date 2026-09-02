import {
    AdmissionHistoryControlOption,
    AdmissionMonitorHistoryControl,
    AdmissionMonitorPolicyAction,
    AdmissionMonitorSeverity,
    AdmissionPolicyActionOption,
    AdmissionSeverityOption
} from '../interfaces';

export const ADMISSION_POLICY_ACTION: AdmissionPolicyActionOption[] = [
    {
        id: AdmissionMonitorPolicyAction.AUDIT,
        localizationKey: 'Admission.Pseudo.PolicyAction.Audit'
    },
    {
        id: AdmissionMonitorPolicyAction.ENFORCE,
        localizationKey: 'Admission.Pseudo.PolicyAction.Enforce'
    }
];

export const ADMISSION_SEVERITY: AdmissionSeverityOption[] = [
    {
        id: AdmissionMonitorSeverity.LOW,
        localizationKey: 'Admission.Pseudo.Severity.Low'
    },
    {
        id: AdmissionMonitorSeverity.MEDIUM,
        localizationKey: 'Admission.Pseudo.Severity.Medium'
    },
    {
        id: AdmissionMonitorSeverity.HIGH,
        localizationKey: 'Admission.Pseudo.Severity.High'
    },
    {
        id: AdmissionMonitorSeverity.CRITICAL,
        localizationKey: 'Admission.Pseudo.Severity.Critical'
    }
];

export const ADMISSION_HISTORY_CONTROL: AdmissionHistoryControlOption[] = [
    {
        id: AdmissionMonitorHistoryControl.ALL,
        localizationKey: 'Admission.Pseudo.HistoryControl.All'
    },
    {
        id: AdmissionMonitorHistoryControl.WITH_THREATS,
        localizationKey: 'Admission.Pseudo.HistoryControl.WithThreats'
    },
    {
        id: AdmissionMonitorHistoryControl.NONE,
        localizationKey: 'Admission.Pseudo.HistoryControl.None'
    }
];
