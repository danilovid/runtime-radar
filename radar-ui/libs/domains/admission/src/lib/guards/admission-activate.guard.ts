import { Store } from '@ngrx/store';
import { inject } from '@angular/core';
import { Observable, filter, map, tap } from 'rxjs';
import { Router, UrlTree } from '@angular/router';

import { LoadStatus, RouterName } from '@cs/core';

import { AdmissionState } from '../interfaces';
import { LOAD_ADMISSION_CONFIG_TODO_ACTION } from '../stores/admission-action.store';
import { getAdmissionLoadStatus } from '../stores/admission-selector.store';

const admissionActivate = (): Observable<boolean | UrlTree> => {
    const router = inject(Router);
    const store = inject<Store<AdmissionState>>(Store);

    return store.select(getAdmissionLoadStatus).pipe(
        tap((status) => {
            if (status === LoadStatus.INIT) {
                store.dispatch(LOAD_ADMISSION_CONFIG_TODO_ACTION());
            }
        }),
        filter((status) => [LoadStatus.LOADED, LoadStatus.ERROR].includes(status)),
        map((status) => status === LoadStatus.LOADED),
        map((isLoaded) => isLoaded || router.createUrlTree([RouterName.ERROR]))
    );
};

export const admissionActivateGuard = () => admissionActivate();

export const admissionActivateChildGuard = () => admissionActivate();
