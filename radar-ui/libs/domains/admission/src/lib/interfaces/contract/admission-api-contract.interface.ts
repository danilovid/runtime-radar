import { AdmissionMonitorConfig } from './admission-monitor-contract.interface';

export interface CreateAdmissionMonitorRequest {
    config: AdmissionMonitorConfig;
}

export type EmptyAdmissionResponse = Record<string, never>;
