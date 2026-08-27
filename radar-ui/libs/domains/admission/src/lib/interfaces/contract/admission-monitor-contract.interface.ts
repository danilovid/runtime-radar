export enum AdmissionMonitorPolicyAction {
    AUDIT = 'AUDIT',
    ENFORCE = 'ENFORCE'
}

export enum AdmissionMonitorSeverity {
    LOW = 'low',
    MEDIUM = 'medium',
    HIGH = 'high',
    CRITICAL = 'critical'
}

export enum AdmissionMonitorHistoryControl {
    NONE = 'NONE',
    ALL = 'ALL',
    WITH_THREATS = 'WITH_THREATS'
}

export type AdmissionMonitorPolicies = {
    [key: string]: AdmissionMonitorPolicy;
};

/** A source, that is a Kyverno policy admission-monitor keeps applied in the cluster. */
export interface AdmissionMonitorPolicy {
    name: string;
    enabled: boolean;
    action: AdmissionMonitorPolicyAction;
    severity: AdmissionMonitorSeverity;
    description?: string;
    yaml?: string;
}

export interface AdmissionMonitorConfig {
    version: string;
    policies: AdmissionMonitorPolicies;
    history_control: AdmissionMonitorHistoryControl;
}

export interface AdmissionMonitor {
    id: string;
    config: AdmissionMonitorConfig;
}
