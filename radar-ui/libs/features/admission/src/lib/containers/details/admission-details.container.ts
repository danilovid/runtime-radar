import { KbqBadgeColors } from '@koobiq/components/badge';
import { ActivatedRoute, Router } from '@angular/router';
import { BehaviorSubject, Observable, catchError, of, switchMap, tap } from 'rxjs';
import { ChangeDetectionStrategy, Component, Input } from '@angular/core';

import { AdmissionEvent, AdmissionRequestService } from '@cs/domains/admission';
import { LoadStatus, RouterName } from '@cs/core';
import { PermissionName, RolePermissionMap } from '@cs/domains/role';

import { AdmissionRouterName } from '../../interfaces/admission-navigation.interface';

@Component({
    templateUrl: './admission-details.container.html',
    styleUrl: './admission-details.container.scss',
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class AdmissionFeatureDetailsContainer {
    // bindToComponentInputs is enabled for the router, so the path parameter arrives as an input
    @Input() set eventId(id: string) {
        this.eventId$.next(id);
    }

    readonly loadStatus$ = new BehaviorSubject<LoadStatus>(LoadStatus.INIT);

    private readonly eventId$ = new BehaviorSubject<string>('');

    readonly event$: Observable<AdmissionEvent | undefined> = this.eventId$.pipe(
        tap(() => this.loadStatus$.next(LoadStatus.IN_PROGRESS)),
        switchMap((id) => this.admissionRequestService.getEvent(id)),
        tap(() => this.loadStatus$.next(LoadStatus.LOADED)),
        catchError(() => {
            this.loadStatus$.next(LoadStatus.ERROR);

            return of(undefined);
        })
    );

    /* eslint @typescript-eslint/dot-notation: "off" */
    readonly permissions: RolePermissionMap = this.route.snapshot.data['permissions'];

    readonly permissionName = PermissionName;

    readonly loadStatus = LoadStatus;

    readonly badgeColors = KbqBadgeColors;

    constructor(
        private readonly admissionRequestService: AdmissionRequestService,
        private readonly route: ActivatedRoute,
        private readonly router: Router
    ) {}

    goBack() {
        this.router.navigate([RouterName.ADMISSION, AdmissionRouterName.EVENTS]);
    }
}
