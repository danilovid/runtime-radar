import { AdmissionEventFilters } from '../interfaces/admission-events.interface';

export const ADMISSION_EVENTS_SLICE_SIZE = 20;

export const ADMISSION_FILTER_INITIAL_STATE: AdmissionEventFilters = {
    resourceKind: [],
    resourceNamespace: [],
    resourceName: [],
    nodeName: [],
    imageNames: [],
    blocked: false,
    hasIncident: false
};
