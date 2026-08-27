import { Action, ActionReducer, createReducer, on } from '@ngrx/store';

import { LoadStatus } from '@cs/core';

import { UPDATE_ADMISSION_STATE_DOC_ACTION } from './admission-action.store';
import { AdmissionConfigStatus, AdmissionMonitorHistoryControl, AdmissionState } from '../interfaces';

const INITIAL_STATE: AdmissionState = {
    loadStatus: LoadStatus.INIT,
    hasChanges: false,
    hasPoliciesChanges: false,
    isExpertMode: false,
    isOverlayed: false,
    configStatus: AdmissionConfigStatus.INIT,
    config: {
        version: '',
        policies: {},
        history_control: AdmissionMonitorHistoryControl.NONE
    }
};

const reducer: ActionReducer<AdmissionState, Action> = createReducer(
    INITIAL_STATE,
    on(UPDATE_ADMISSION_STATE_DOC_ACTION, (state, values) => ({ ...state, ...values }))
);

export function admissionReducer(state: AdmissionState | undefined, action: Action): AdmissionState {
    return reducer(state, action);
}
