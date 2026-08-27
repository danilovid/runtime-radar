import { DateAdapter } from '@koobiq/components/core';
import { DateTime } from 'luxon';
import { KbqBadgeColors } from '@koobiq/components/badge';
import { ActivatedRoute, Router } from '@angular/router';
import { BehaviorSubject, Observable, catchError, map, of, switchMap, tap } from 'rxjs';
import { ChangeDetectionStrategy, Component } from '@angular/core';

import { ApiPathService } from '@cs/api';
import {
    AdmissionEventCursorDirection,
    AdmissionRequestService,
    GetAdmissionEventsResponse
} from '@cs/domains/admission';
import { ClusterStoreService, RegisteredCluster } from '@cs/domains/cluster';
import { LoadStatus, RouterName } from '@cs/core';
import { PermissionName, RolePermissionMap } from '@cs/domains/role';

import { ADMISSION_NAVIGATION_TABS } from '../../constants/admission-navigation.constant';
import { AdmissionRouterName } from '../../interfaces/admission-navigation.interface';
import { AdmissionFeatureFilterService as filterUtils } from '../../services/admission-filter.service';
import { ADMISSION_EVENTS_SLICE_SIZE, ADMISSION_FILTER_INITIAL_STATE } from '../../constants/admission-filter.constant';
import { AdmissionEventFilters, AdmissionEventsPagination } from '../../interfaces/admission-events.interface';

const EMPTY_RESPONSE: GetAdmissionEventsResponse = {
    admission_events: [],
    left_cursor: '',
    right_cursor: ''
};

@Component({
    templateUrl: './admission-events.container.html',
    styleUrl: './admission-events.container.scss',
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class AdmissionFeatureEventsContainer {
    readonly clusters$: Observable<RegisteredCluster[]> = this.clusterStoreService.registeredClusters$;

    readonly activeClusterHost$ = this.apiPathService.host$;

    readonly loadStatus$ = new BehaviorSubject<LoadStatus>(LoadStatus.INIT);

    readonly filters$ = new BehaviorSubject<AdmissionEventFilters>({ ...ADMISSION_FILTER_INITIAL_STATE });

    readonly activeCursor$ = new BehaviorSubject<AdmissionEventsPagination>({
        direction: AdmissionEventCursorDirection.RIGHT,
        cursor: this.dateAdapter.today().toJSDate().toISOString() // RFC3339
    });

    readonly isFilterExist$: Observable<boolean> = this.filters$.pipe(map((f) => filterUtils.isFilterExist(f)));

    readonly eventsResponse$: Observable<GetAdmissionEventsResponse> = this.activeCursor$.pipe(
        tap(() => this.loadStatus$.next(LoadStatus.IN_PROGRESS)),
        switchMap((pagination) => {
            const filters = this.filters$.value;
            const request = { cursor: pagination.cursor, slice_size: ADMISSION_EVENTS_SLICE_SIZE };

            if (!filterUtils.isFilterExist(filters)) {
                return this.admissionRequestService.getEvents(pagination.direction, request);
            }

            return this.admissionRequestService.getEventsByFilter(pagination.direction, {
                ...request,
                filter: filterUtils.convertFiltersToRequestNode(filters)
            });
        }),
        tap(() => this.loadStatus$.next(LoadStatus.LOADED)),
        catchError(() => {
            this.loadStatus$.next(LoadStatus.ERROR);

            return of(EMPTY_RESPONSE);
        })
    );

    /* eslint @typescript-eslint/dot-notation: "off" */
    readonly permissions: RolePermissionMap = this.route.snapshot.data['permissions'];

    readonly permissionName = PermissionName;

    readonly admissionNavigationTabs = ADMISSION_NAVIGATION_TABS;

    readonly admissionRouterName = AdmissionRouterName;

    readonly admissionEventCursorDirection = AdmissionEventCursorDirection;

    readonly loadStatus = LoadStatus;

    readonly badgeColors = KbqBadgeColors;

    constructor(
        private readonly apiPathService: ApiPathService,
        private readonly clusterStoreService: ClusterStoreService,
        private readonly dateAdapter: DateAdapter<DateTime>,
        private readonly admissionRequestService: AdmissionRequestService,
        private readonly route: ActivatedRoute,
        private readonly router: Router
    ) {}

    tabChange(path?: string) {
        this.router.navigate([RouterName.ADMISSION, path]);
    }

    changeFilter(filters: AdmissionEventFilters) {
        this.filters$.next(filters);
        this.goToStartPage();
    }

    changePage(direction: AdmissionEventCursorDirection, cursor: string) {
        this.activeCursor$.next({ direction, cursor });
    }

    goToStartPage() {
        this.activeCursor$.next({
            direction: AdmissionEventCursorDirection.RIGHT,
            cursor: this.dateAdapter.today().toJSDate().toISOString()
        });
    }

    openEvent(id: string) {
        this.router.navigate([RouterName.ADMISSION, AdmissionRouterName.EVENTS, id]);
    }

    switchCluster(id: string) {
        this.clusterStoreService.switchCluster(id);
    }
}
