import { DateTime } from 'luxon';
import { KbqBadgeColors } from '@koobiq/components/badge';
import { ActivatedRoute, Router } from '@angular/router';
import { BehaviorSubject, Observable, catchError, map, of, switchMap, tap } from 'rxjs';
import { ChangeDetectionStrategy, Component, Input } from '@angular/core';

import { I18nService } from '@cs/i18n';
import { IntegrationStoreService } from '@cs/domains/integration';
import { AdmissionEvent, AdmissionRequestService } from '@cs/domains/admission';
import { AssistantEventKind, AssistantMode, AssistantStoreService } from '@cs/domains/assistant';
import { LoadStatus, RouterName } from '@cs/core';
import { PermissionName, RolePermissionMap } from '@cs/domains/role';

import { AdmissionRouterName } from '../../interfaces/admission-navigation.interface';

@Component({
    templateUrl: './admission-details.container.html',
    styleUrl: './admission-details.container.scss',
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class AdmissionFeatureDetailsContainer {
    readonly dateTimeFullFormat = DateTime.DATETIME_FULL;

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

    /** The buttons only make sense once a model can answer. */
    readonly hasIntegration$: Observable<boolean> = this.integrationStoreService.aiIntegrations$.pipe(
        map((integrations) => !!integrations.length)
    );

    constructor(
        private readonly admissionRequestService: AdmissionRequestService,
        private readonly assistantStoreService: AssistantStoreService,
        private readonly i18nService: I18nService,
        private readonly integrationStoreService: IntegrationStoreService,
        private readonly route: ActivatedRoute,
        private readonly router: Router
    ) {}

    goBack() {
        this.router.navigate([RouterName.ADMISSION, AdmissionRouterName.EVENTS]);
    }

    /**
     * Explains the finding in the assistant panel. The finding itself is not
     * handed over: the assistant reads it with its own tools, so the model sees
     * what the system recorded rather than what this page happens to hold.
     */
    explainEvent(event: AdmissionEvent) {
        this.assistantStoreService.open({
            eventId: event.id,
            eventKind: AssistantEventKind.ADMISSION,
            question: this.i18nService.translate('Admission.DetailsPage.Text.ExplainEvent'),
            mode: AssistantMode.EXPLAIN
        });
    }

    /** Opens the assistant on this finding without asking anything yet. */
    askAssistant(event: AdmissionEvent) {
        this.assistantStoreService.open({
            eventId: event.id,
            eventKind: AssistantEventKind.ADMISSION
        });
    }
}
