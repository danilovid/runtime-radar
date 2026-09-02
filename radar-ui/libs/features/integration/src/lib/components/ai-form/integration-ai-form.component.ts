import { HttpErrorResponse } from '@angular/common/http';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import {
    AfterViewInit,
    ChangeDetectionStrategy,
    Component,
    DestroyRef,
    EventEmitter,
    Input,
    OnInit,
    Output
} from '@angular/core';
import { BehaviorSubject, Observable, debounceTime, distinctUntilChanged, map, startWith, tap } from 'rxjs';
import { FormBuilder, FormGroup, ValidatorFn, Validators } from '@angular/forms';
import { KbqToastService, KbqToastStyle } from '@koobiq/components/toast';

import { I18nService } from '@cs/i18n';
import { ApiErrorCode, ApiUtilsService as apiUtils } from '@cs/api';
import {
    CoreValidators,
    FORM_VALIDATION_DENIED_IP,
    FORM_VALIDATION_REG_EXP,
    FormScheme,
    CoreUtilsService as utils
} from '@cs/core';
import {
    INTEGRATION_AI_PROVIDER_TYPE,
    IntegrationAI,
    IntegrationAIProviderType,
    IntegrationAIRequestService,
    IntegrationAIScope,
    IntegrationType
} from '@cs/domains/integration';

import { IntegrationAIForm, IntegrationAIScopeOption } from '../../interfaces/integration-form.interface';

// The halves of the product the assistant can be limited to. Leaving both
// unchecked offers it every tool the signed-in user's role allows, which is
// what an integration created before scopes existed does.
const INTEGRATION_AI_SCOPES: IntegrationAIScopeOption[] = [
    {
        id: IntegrationAIScope.RUNTIME_MONITOR,
        localizationKey: 'Integration.Pseudo.Scope.RuntimeMonitor',
        descriptionLocalizationKey: 'Integration.Pseudo.Scope.RuntimeMonitorDescription'
    },
    {
        id: IntegrationAIScope.ADMISSION,
        localizationKey: 'Integration.Pseudo.Scope.Admission',
        descriptionLocalizationKey: 'Integration.Pseudo.Scope.AdmissionDescription'
    }
];

// An empty base url is resolved by the backend to the public OpenAI API, so the
// openai-compatible provider has to name its endpoint explicitly. Anthropic and
// Ollama each have a single meaningful default and may leave it blank.
function baseUrlValidators(provider: IntegrationAIProviderType | null): ValidatorFn[] {
    const validators: ValidatorFn[] = [
        Validators.pattern(FORM_VALIDATION_REG_EXP.API_BASE_URL),
        CoreValidators.isIpSegmentAllowed(FORM_VALIDATION_DENIED_IP.LOCALHOST)
    ];

    if (provider === IntegrationAIProviderType.OPENAI_COMPATIBLE) {
        validators.push(Validators.required);
    }

    return validators;
}

@Component({
    selector: 'cs-integration-feature-ai-form-component',
    templateUrl: './integration-ai-form.component.html',
    styleUrl: './integration-ai-form.component.scss',
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class IntegrationFeatureAIFormComponent implements AfterViewInit, OnInit {
    @Input() values?: IntegrationAI;

    @Output() formChange = new EventEmitter<IntegrationAIForm | undefined>();

    readonly isTesting$ = new BehaviorSubject(false);

    readonly form: FormGroup<FormScheme<IntegrationAIForm>> = this.formBuilder.group({
        name: ['', Validators.required],
        provider: [IntegrationAIProviderType.OPENAI_COMPATIBLE, Validators.required],
        baseUrl: ['', baseUrlValidators(IntegrationAIProviderType.OPENAI_COMPATIBLE)],
        model: ['', Validators.required],
        apiKey: [''],
        ca: [''],
        isLocal: [false],
        isInsecure: [false],
        scopes: [[] as IntegrationAIScope[]]
    });

    readonly providerOptions = INTEGRATION_AI_PROVIDER_TYPE;

    readonly scopeOptions = INTEGRATION_AI_SCOPES;

    private readonly onFormValidChanges$: Observable<boolean> = this.form.valueChanges.pipe(
        startWith(this.form.value),
        debounceTime(250),
        distinctUntilChanged(),
        map(() => utils.isFormValid(this.form.controls)),
        tap((isValid) => {
            if (isValid) {
                const formValues = utils.getFormValues<IntegrationAIForm>(this.form.controls);
                this.formChange.emit(utils.getTrimmedFormValues<IntegrationAIForm>(formValues));
            } else {
                this.formChange.emit(undefined);
            }
        })
    );

    constructor(
        private readonly destroyRef: DestroyRef,
        private readonly formBuilder: FormBuilder,
        private readonly i18nService: I18nService,
        private readonly integrationAIRequestService: IntegrationAIRequestService,
        private readonly toastService: KbqToastService
    ) {}

    ngOnInit() {
        this.onFormValidChanges$.pipe(takeUntilDestroyed(this.destroyRef)).subscribe();

        this.form.controls.provider.valueChanges
            .pipe(
                startWith(this.form.controls.provider.value),
                distinctUntilChanged(),
                takeUntilDestroyed(this.destroyRef)
            )
            .subscribe((provider) => {
                const control = this.form.controls.baseUrl;

                control.setValidators(baseUrlValidators(provider));
                control.updateValueAndValidity();
            });
    }

    /** Whether the assistant is limited to the given half of the product. */
    isScopeSelected(scope: IntegrationAIScope): boolean {
        return (this.form.controls.scopes.value ?? []).includes(scope);
    }

    // The checkboxes carry a list rather than a boolean each, so the control is
    // maintained by hand: there is no form control for a single scope to bind.
    toggleScope(scope: IntegrationAIScope) {
        const control = this.form.controls.scopes;
        const selected = control.value ?? [];

        control.setValue(selected.includes(scope) ? selected.filter((item) => item !== scope) : [...selected, scope]);
    }

    ngAfterViewInit() {
        if (this.values) {
            this.form.patchValue({
                name: this.values.name,
                provider: this.values.ai.provider,
                baseUrl: this.values.ai.base_url,
                model: this.values.ai.model,
                ca: this.values.ai.ca,
                isLocal: this.values.ai.is_local,
                isInsecure: !this.values.ai.insecure,
                scopes: this.values.ai.scopes || []
            });
        }
    }

    testConnection() {
        if (!utils.isFormValid(this.form.controls)) {
            return;
        }

        this.isTesting$.next(true);

        const formValues = utils.getTrimmedFormValues<IntegrationAIForm>(
            utils.getFormValues<IntegrationAIForm>(this.form.controls)
        );

        this.integrationAIRequestService
            .testAIIntegration({
                integration: {
                    type: this.values?.type || IntegrationType.AI,
                    name: formValues.name,
                    skip_check: false,
                    ai: {
                        provider: formValues.provider,
                        base_url: formValues.baseUrl,
                        model: formValues.model,
                        api_key: formValues.apiKey,
                        ca: formValues.ca,
                        is_local: formValues.isLocal,
                        insecure: !formValues.isInsecure
                    }
                }
            })
            .pipe(takeUntilDestroyed(this.destroyRef))
            .subscribe({
                next: () => {
                    this.isTesting$.next(false);
                    this.toastService.show({
                        style: KbqToastStyle.Success,
                        title: this.i18nService.translate('Integration.Pseudo.Notification.AITestSucceeded')
                    });
                },
                error: (error: HttpErrorResponse) => {
                    this.isTesting$.next(false);

                    if (apiUtils.getReasonCode(error) === ApiErrorCode.INTEGRATION_INACCESSIBLE) {
                        this.toastService.show({
                            style: KbqToastStyle.Warning,
                            title: this.i18nService.translate('Integration.Pseudo.Notification.IntegrationInaccessible')
                        });

                        return;
                    }

                    this.toastService.show({
                        style: KbqToastStyle.Warning,
                        title: this.i18nService.translate('Integration.Pseudo.Notification.AITestFailed')
                    });
                }
            });
    }
}
