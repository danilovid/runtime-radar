import { AdmissionEventCursorDirection, AdmissionFilterRequest } from '@cs/domains/admission';

export interface AdmissionEventsPagination {
    direction: AdmissionEventCursorDirection;
    cursor: string; // RFC3339
}

/**
 * AdmissionEventFilters is the filter as the user edits it. It is converted into
 * AdmissionFilterRequest before being sent, dropping everything that is left empty.
 */
export interface AdmissionEventFilters {
    resourceKind: string[];
    resourceNamespace: string[];
    resourceName: string[];
    nodeName: string[];
    imageNames: string[];
    // Both flags are plain toggles: switched off means the filter is not applied at all,
    // there is no reason to look for requests that were explicitly not blocked.
    blocked: boolean;
    hasIncident: boolean;
}

export type AdmissionFilterRequestNode = Partial<AdmissionFilterRequest>;
