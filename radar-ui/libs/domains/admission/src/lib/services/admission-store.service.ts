import { Injectable } from '@angular/core';
import { Observable } from 'rxjs';
import { Store } from '@ngrx/store';

import { AdmissionMonitorConfig, AdmissionState } from '../interfaces';
import {
    CHECK_ADMISSION_CHANGES_TODO_ACTION,
    CREATE_ADMISSION_CONFIG_TODO_ACTION,
    HIDE_ADMISSION_OVERLAY_TODO_ACTION,
    SWITCH_ADMISSION_EXPERT_MODE_TODO_ACTION
} from '../stores/admission-action.store';
import {
    getAdmissionHasChanges,
    getAdmissionHasPoliciesChanges,
    getAdmissionIsExpertMode,
    getAdmissionIsOverlayed,
    getAdmissionMonitorConfig
} from '../stores/admission-selector.store';

@Injectable({
    providedIn: 'root'
})
export class AdmissionStoreService {
    readonly admissionMonitorConfig$: Observable<AdmissionMonitorConfig> = this.store.select(getAdmissionMonitorConfig);

    readonly admissionHasChanges$: Observable<boolean> = this.store.select(getAdmissionHasChanges);

    readonly admissionHasPoliciesChanges$: Observable<boolean> = this.store.select(getAdmissionHasPoliciesChanges);

    readonly admissionIsExpertMode$: Observable<boolean> = this.store.select(getAdmissionIsExpertMode);

    readonly admissionIsOverlayed$: Observable<boolean> = this.store.select(getAdmissionIsOverlayed);

    constructor(private readonly store: Store<AdmissionState>) {}

    createConfig(config: AdmissionMonitorConfig) {
        this.store.dispatch(CREATE_ADMISSION_CONFIG_TODO_ACTION({ config }));
    }

    checkChanges(config: AdmissionMonitorConfig) {
        this.store.dispatch(CHECK_ADMISSION_CHANGES_TODO_ACTION({ config }));
    }

    switchExpertMode() {
        this.store.dispatch(SWITCH_ADMISSION_EXPERT_MODE_TODO_ACTION());
    }

    hideOverlay() {
        this.store.dispatch(HIDE_ADMISSION_OVERLAY_TODO_ACTION());
    }
}
