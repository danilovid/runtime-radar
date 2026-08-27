import { Store } from '@ngrx/store';
import { inject } from '@angular/core';
import { Observable, filter, map, tap } from 'rxjs';

import { LoadStatus } from '@cs/core';

import { AdmissionState } from '../interfaces';
import { DEACTIVATE_ADMISSION_CONFIG_TODO_ACTION } from '../stores/admission-action.store';
import { getAdmissionLoadStatus } from '../stores/admission-selector.store';

const admissionDeactivate = (): Observable<boolean> => {
    const store = inject<Store<AdmissionState>>(Store);

    return store.select(getAdmissionLoadStatus).pipe(
        tap((status) => {
            if ([LoadStatus.LOADED, LoadStatus.ERROR].includes(status)) {
                store.dispatch(DEACTIVATE_ADMISSION_CONFIG_TODO_ACTION());
            }
        }),
        filter((status) => status === LoadStatus.INIT),
        map((status) => status === LoadStatus.INIT)
    );
};

export const admissionDeactivateGuard = () => admissionDeactivate();
