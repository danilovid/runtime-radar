import { LoadStatus } from '@cs/core';

import { AdmissionMonitorConfig } from '../contract/admission-monitor-contract.interface';

export enum AdmissionConfigStatus {
    INIT = 'INIT',
    STAY = 'STAY',
    MODIFY = 'MODIFY'
}

export interface AdmissionState {
    loadStatus: LoadStatus;
    hasChanges: boolean;
    hasPoliciesChanges: boolean;
    configStatus: AdmissionConfigStatus;
    isExpertMode: boolean;
    isOverlayed: boolean;
    config: AdmissionMonitorConfig;
}
