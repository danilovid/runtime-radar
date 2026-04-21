import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { HttpErrorResponse } from '@angular/common/http';
import {
    ChangeDetectionStrategy,
    Component,
    DestroyRef,
    Input,
    OnInit
} from '@angular/core';
import { FormBuilder, FormGroup, Validators } from '@angular/forms';
import { BehaviorSubject, catchError, of, take, tap } from 'rxjs';
import { KbqModalRef } from '@koobiq/components/modal';
import { KbqToastService, KbqToastStyle } from '@koobiq/components/toast';

import { ApiErrorCode, ApiUtilsService as apiUtils } from '@cs/api';
import {
    ExplainRuntimeEventResponse,
    IntegrationAI,
    IntegrationAIRequestService
} from '@cs/domains/integration';
import { FormScheme } from '@cs/core';
import { I18nService } from '@cs/i18n';
import { RuntimeEvent } from '@cs/domains/runtime';

interface RuntimeExplainEventForm {
    integrationId: string;
}

@Component({
    templateUrl: './runtime-explain-event-modal.component.html',
    styles: [
        `
            .runtime-feature-explain-event-modal-empty {
                color: var(--kbq-foreground-contrast-secondary);
            }

            .runtime-feature-explain-event-modal-result {
                display: grid;
                gap: 16px;
            }

            .runtime-feature-explain-event-modal-result dl {
                margin: 0;
            }

            .runtime-feature-explain-event-modal-result pre {
                margin: 0;
                white-space: pre-wrap;
                word-break: break-word;
            }
        `
    ],
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class RuntimeFeatureExplainEventModalComponent implements OnInit {
    @Input() event?: RuntimeEvent;

    readonly form: FormGroup<FormScheme<RuntimeExplainEventForm>> = this.formBuilder.group({
        integrationId: ['', Validators.required]
    });

    readonly integrations$ = new BehaviorSubject<IntegrationAI[]>([]);

    readonly isLoading$ = new BehaviorSubject(true);

    readonly isRunning$ = new BehaviorSubject(false);

    readonly result$ = new BehaviorSubject<ExplainRuntimeEventResponse | undefined>(undefined);

    constructor(
        private readonly destroyRef: DestroyRef,
        private readonly formBuilder: FormBuilder,
        private readonly i18nService: I18nService,
        private readonly integrationAIRequestService: IntegrationAIRequestService,
        private readonly modal: KbqModalRef,
        private readonly toastService: KbqToastService
    ) {}

    ngOnInit() {
        this.integrationAIRequestService
            .getAIIntegrations()
            .pipe(
                take(1),
                tap((integrations) => {
                    this.integrations$.next(integrations);

                    if (integrations.length) {
                        this.form.patchValue({
                            integrationId: integrations[0].id
                        });
                    }

                    this.isLoading$.next(false);
                }),
                catchError(() => {
                    this.isLoading$.next(false);
                    this.integrations$.next([]);

                    this.toastService.show({
                        style: KbqToastStyle.Warning,
                        title: this.i18nService.translate('Runtime.Pseudo.Notification.ExplainFailed')
                    });

                    return of([]);
                }),
                takeUntilDestroyed(this.destroyRef)
            )
            .subscribe();
    }

    run() {
        const integrationId = this.form.get('integrationId')?.value;
        if (!integrationId || !this.event) {
            return;
        }

        this.isRunning$.next(true);
        this.result$.next(undefined);

        this.integrationAIRequestService
            .explainRuntimeEvent({
                integration_id: integrationId,
                event_id: this.event.id,
                event_json: JSON.stringify(this.event)
            })
            .pipe(take(1), takeUntilDestroyed(this.destroyRef))
            .subscribe({
                next: (result) => {
                    this.isRunning$.next(false);
                    this.result$.next(result);
                },
                error: (error: HttpErrorResponse) => {
                    this.isRunning$.next(false);

                    const title =
                        apiUtils.getReasonCode(error) === ApiErrorCode.INTEGRATION_INACCESSIBLE
                            ? 'Integration.Pseudo.Notification.IntegrationInaccessible'
                            : 'Runtime.Pseudo.Notification.ExplainFailed';

                    this.toastService.show({
                        style: KbqToastStyle.Warning,
                        title: this.i18nService.translate(title)
                    });
                }
            });
    }

    close() {
        this.modal.destroy();
    }
}
