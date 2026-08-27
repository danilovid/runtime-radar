import { ActionReducerMap, createFeatureSelector, createSelector } from '@ngrx/store';

import { AdmissionState } from '../interfaces';
import { admissionReducer } from './admission-reducer.store';

export const ADMISSION_DOMAIN_KEY = 'admission';

export interface AdmissionDomainState {
    readonly domain: AdmissionState;
}

const selectAdmissionDomainState = createFeatureSelector<AdmissionDomainState>(ADMISSION_DOMAIN_KEY);
const selectAdmissionState = createSelector(selectAdmissionDomainState, (state: AdmissionDomainState) => state.domain);

export const getAdmissionLoadStatus = createSelector(selectAdmissionState, (state: AdmissionState) => state.loadStatus);

export const getAdmissionMonitorConfig = createSelector(selectAdmissionState, (state: AdmissionState) => state.config);

export const getAdmissionHasChanges = createSelector(selectAdmissionState, (state: AdmissionState) => state.hasChanges);

export const getAdmissionHasPoliciesChanges = createSelector(
    selectAdmissionState,
    (state: AdmissionState) => state.hasPoliciesChanges
);

export const getAdmissionIsExpertMode = createSelector(
    selectAdmissionState,
    (state: AdmissionState) => state.isExpertMode
);

export const getAdmissionIsOverlayed = createSelector(
    selectAdmissionState,
    (state: AdmissionState) => state.isOverlayed
);

export const getAdmissionConfigStatus = createSelector(
    selectAdmissionState,
    (state: AdmissionState) => state.configStatus
);

export const admissionDomainReducer: ActionReducerMap<AdmissionDomainState> = {
    domain: admissionReducer
};
