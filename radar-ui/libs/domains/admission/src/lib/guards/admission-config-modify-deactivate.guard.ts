import { Store } from '@ngrx/store';
import { inject } from '@angular/core';
import { Observable, filter, map, tap } from 'rxjs';

import { UPDATE_ADMISSION_STATE_DOC_ACTION } from '../stores/admission-action.store';
import { getAdmissionConfigStatus } from '../stores/admission-selector.store';
import { AdmissionConfigStatus, AdmissionState } from '../interfaces';

const admissionConfigModifyDeactivate = (): Observable<boolean> => {
    const store = inject<Store<AdmissionState>>(Store);

    return store.select(getAdmissionConfigStatus).pipe(
        tap((status) => {
            if (status === AdmissionConfigStatus.MODIFY) {
                store.dispatch(UPDATE_ADMISSION_STATE_DOC_ACTION({ isOverlayed: true }));
            }
        }),
        map((status) => status === AdmissionConfigStatus.STAY),
        filter((isNavigateAllowed) => isNavigateAllowed),
        tap(() => {
            store.dispatch(
                UPDATE_ADMISSION_STATE_DOC_ACTION({ configStatus: AdmissionConfigStatus.INIT, isOverlayed: false })
            );
        })
    );
};

export const admissionConfigModifyDeactivateGuard = () => admissionConfigModifyDeactivate();
