import { Injectable } from '@angular/core';
import { Action, Store } from '@ngrx/store';
import { Actions, createEffect, ofType } from '@ngrx/effects';
import { KbqToastService, KbqToastStyle } from '@koobiq/components/toast';
import { Observable, of } from 'rxjs';
import { catchError, filter, map, switchMap, take, tap } from 'rxjs/operators';

import { I18nService } from '@cs/i18n';
import { SIGN_OUT_EVENT_ACTION } from '@cs/domains/auth';
import { SWITCH_CLUSTER_EVENT_ACTION } from '@cs/domains/cluster';
import { CoreWindowService, LoadStatus, CoreUtilsService as utils } from '@cs/core';

import { AdmissionRequestService } from '../services/admission-request.service';
import { AdmissionHelperService as admissionHelper } from '../services/admission-helper.service';
import { AdmissionConfigStatus, AdmissionState } from '../interfaces';
import {
    CHECK_ADMISSION_CHANGES_TODO_ACTION,
    CREATE_ADMISSION_CONFIG_TODO_ACTION,
    DEACTIVATE_ADMISSION_CONFIG_TODO_ACTION,
    HIDE_ADMISSION_OVERLAY_TODO_ACTION,
    LOAD_ADMISSION_CONFIG_TODO_ACTION,
    SWITCH_ADMISSION_EXPERT_MODE_TODO_ACTION,
    UPDATE_ADMISSION_STATE_DOC_ACTION
} from './admission-action.store';
import {
    getAdmissionIsExpertMode,
    getAdmissionLoadStatus,
    getAdmissionMonitorConfig
} from './admission-selector.store';

const ADMISSION_EXPERT_MODE_LOCAL_STORAGE_KEY = 'xprtmd-adm';

@Injectable({
    providedIn: 'root'
})
export class AdmissionEffectStore {
    readonly checkExpertMode$: Observable<Action> = createEffect(() =>
        this.actions$.pipe(
            ofType(LOAD_ADMISSION_CONFIG_TODO_ACTION),
            map(() => this.coreWindowService.localStorage.getItem(ADMISSION_EXPERT_MODE_LOCAL_STORAGE_KEY)),
            map((value) =>
                UPDATE_ADMISSION_STATE_DOC_ACTION({
                    isExpertMode: value ? value === 'true' : false
                })
            )
        )
    );

    readonly loadConfig$: Observable<Action> = createEffect(() =>
        this.actions$.pipe(
            ofType(LOAD_ADMISSION_CONFIG_TODO_ACTION),
            switchMap(() =>
                this.admissionRequestService.getAdmissionMonitor().pipe(
                    map((response) => response.config),
                    catchError(() => of(undefined))
                )
            ),
            map((config) => {
                if (config === undefined) {
                    return UPDATE_ADMISSION_STATE_DOC_ACTION({
                        loadStatus: LoadStatus.ERROR
                    });
                }

                return UPDATE_ADMISSION_STATE_DOC_ACTION({
                    loadStatus: LoadStatus.LOADED,
                    config
                });
            })
        )
    );

    readonly reloadConfig$: Observable<Action> = createEffect(() =>
        this.actions$.pipe(
            ofType(SWITCH_CLUSTER_EVENT_ACTION),
            switchMap(() => this.store.select(getAdmissionLoadStatus).pipe(take(1))),
            filter((status) => status !== LoadStatus.INIT),
            map(() => LOAD_ADMISSION_CONFIG_TODO_ACTION())
        )
    );

    readonly deactivateState$: Observable<Action> = createEffect(() =>
        this.actions$.pipe(
            ofType(DEACTIVATE_ADMISSION_CONFIG_TODO_ACTION),
            map(() =>
                UPDATE_ADMISSION_STATE_DOC_ACTION({
                    loadStatus: LoadStatus.INIT
                })
            )
        )
    );

    readonly createAdmissionMonitor$: Observable<Action> = createEffect(() =>
        this.actions$.pipe(
            ofType(CREATE_ADMISSION_CONFIG_TODO_ACTION),
            switchMap((action) =>
                this.admissionRequestService.createAdmissionMonitor(action.config).pipe(
                    take(1),
                    map((response) => response.config)
                )
            ),
            filter((config) => config && !!Object.keys(config).length),
            map((config) =>
                UPDATE_ADMISSION_STATE_DOC_ACTION({
                    config,
                    configStatus: AdmissionConfigStatus.STAY
                })
            ),
            tap(() => {
                this.toastService.show({
                    style: KbqToastStyle.Success,
                    title: this.i18nService.translate('Admission.Pseudo.Notification.Created')
                });
            })
        )
    );

    readonly checkChanges$: Observable<Action> = createEffect(() =>
        this.actions$.pipe(
            ofType(CHECK_ADMISSION_CHANGES_TODO_ACTION),
            switchMap((action) =>
                this.store.select(getAdmissionMonitorConfig).pipe(
                    take(1),
                    map((config) => ({
                        previous: admissionHelper.convertConfigToDiffValues(config),
                        current: action.config
                    }))
                )
            ),
            map(({ previous, current }) => {
                const hasChanges = !utils.isEqual(previous, current);

                return UPDATE_ADMISSION_STATE_DOC_ACTION({
                    hasChanges,
                    // A change of the manifest itself, unlike enabling or disabling a source, means
                    // the policy set in the cluster is rebuilt, which is worth warning about.
                    hasPoliciesChanges: !utils.isEqual(
                        Object.entries(previous.policies).map(([key, { enabled, ...rest }]) => [key, rest]),
                        Object.entries(current.policies).map(([key, { enabled, ...rest }]) => [key, rest])
                    ),
                    configStatus: hasChanges ? AdmissionConfigStatus.MODIFY : AdmissionConfigStatus.STAY
                });
            })
        )
    );

    readonly switchExpertMode$: Observable<Action> = createEffect(() =>
        this.actions$.pipe(
            ofType(SWITCH_ADMISSION_EXPERT_MODE_TODO_ACTION),
            switchMap(() => this.store.select(getAdmissionIsExpertMode).pipe(take(1))),
            tap((isExpertMode) => {
                this.coreWindowService.localStorage.setItem(
                    ADMISSION_EXPERT_MODE_LOCAL_STORAGE_KEY,
                    (!isExpertMode).toString()
                );
            }),
            map((isExpertMode) => UPDATE_ADMISSION_STATE_DOC_ACTION({ isExpertMode: !isExpertMode }))
        )
    );

    readonly deactivateExpertMode$: Observable<Action> = createEffect(() =>
        this.actions$.pipe(
            ofType(SIGN_OUT_EVENT_ACTION),
            map(() => UPDATE_ADMISSION_STATE_DOC_ACTION({ isExpertMode: false })),
            tap(() => {
                this.coreWindowService.localStorage.removeItem(ADMISSION_EXPERT_MODE_LOCAL_STORAGE_KEY);
            })
        )
    );

    readonly hideOverlay$: Observable<Action> = createEffect(() =>
        this.actions$.pipe(
            ofType(HIDE_ADMISSION_OVERLAY_TODO_ACTION),
            map(() =>
                UPDATE_ADMISSION_STATE_DOC_ACTION({
                    isOverlayed: false
                })
            )
        )
    );

    constructor(
        private readonly actions$: Actions,
        private readonly i18nService: I18nService,
        private readonly coreWindowService: CoreWindowService,
        private readonly admissionRequestService: AdmissionRequestService,
        private readonly toastService: KbqToastService,
        private readonly store: Store<AdmissionState>
    ) {}
}
