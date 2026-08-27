import { createAction, props } from '@ngrx/store';

import { AdmissionMonitorConfig, AdmissionState } from '../interfaces';

export const LOAD_ADMISSION_CONFIG_TODO_ACTION = createAction('[Admission] Load Config');

export const DEACTIVATE_ADMISSION_CONFIG_TODO_ACTION = createAction('[Admission] Deactivate Config');

export const CREATE_ADMISSION_CONFIG_TODO_ACTION = createAction(
    '[Admission] Create Config',
    props<{ config: AdmissionMonitorConfig }>()
);

export const CHECK_ADMISSION_CHANGES_TODO_ACTION = createAction(
    '[Admission] Check Changes',
    props<{ config: AdmissionMonitorConfig }>()
);

export const SWITCH_ADMISSION_EXPERT_MODE_TODO_ACTION = createAction('[Admission] Switch Expert Mode');

export const HIDE_ADMISSION_OVERLAY_TODO_ACTION = createAction('[Admission] Hide Overlay');

export const UPDATE_ADMISSION_STATE_DOC_ACTION = createAction(
    '[Admission] (Doc) Update State',
    props<Partial<AdmissionState>>()
);
