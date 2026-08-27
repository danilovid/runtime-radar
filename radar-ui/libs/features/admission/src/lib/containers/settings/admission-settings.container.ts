import { KbqAlertColors } from '@koobiq/components/alert';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { ActivatedRoute, Router } from '@angular/router';
import { ChangeDetectionStrategy, ChangeDetectorRef, Component, DestroyRef, OnDestroy, OnInit } from '@angular/core';
import { FormBuilder, FormGroup, FormRecord } from '@angular/forms';
import { KbqSidepanelConfig, KbqSidepanelPosition, KbqSidepanelService } from '@koobiq/components/sidepanel';
import { Observable, debounceTime, distinctUntilChanged, filter, map, startWith, take, tap } from 'rxjs';

import { ApiPathService } from '@cs/api';
import { I18nService } from '@cs/i18n';
import { SharedModalService } from '@cs/shared';
import {
    ADMISSION_HISTORY_CONTROL,
    ADMISSION_POLICY_ACTION,
    ADMISSION_SEVERITY,
    AdmissionMonitorConfig,
    AdmissionMonitorHistoryControl,
    AdmissionMonitorPolicies,
    AdmissionMonitorPolicy,
    AdmissionMonitorPolicyAction,
    AdmissionMonitorSeverity,
    AdmissionStoreService
} from '@cs/domains/admission';
import { ClusterStoreService, RegisteredCluster } from '@cs/domains/cluster';
import { FormScheme, RouterName, CoreUtilsService as utils } from '@cs/core';
import { PermissionName, PermissionType, RolePermissionMap } from '@cs/domains/role';

import { ADMISSION_NAVIGATION_TABS } from '../../constants/admission-navigation.constant';
import { AdmissionFeatureSidepanelPolicyComponent } from '../../components/sidepanel-policy/admission-sidepanel-policy.component';
import { AdmissionFeatureSidepanelPolicyFormComponent } from '../../components/sidepanel-policy-form/admission-sidepanel-policy-form.component';
import { AdmissionFeatureConfigUtilsService as admissionConfigUtils } from '../../services/admission-utils.service';

import {
    AdmissionExpertModeForm,
    AdmissionSettingForm,
    AdmissionSettingPolicyForm,
    AdmissionSettingPolicyRecord
} from '../../interfaces/admission-form.interface';
import {
    AdmissionSidepanelPolicyFormProps,
    AdmissionSidepanelPolicyProps
} from '../../interfaces/admission-sidepanel.interface';

@Component({
    templateUrl: './admission-settings.container.html',
    styleUrl: './admission-settings.container.scss',
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class AdmissionFeatureSettingsContainer implements OnInit, OnDestroy {
    readonly expertModeForm: FormGroup<FormScheme<AdmissionExpertModeForm>> = this.formBuilder.group({
        isExpert: [false]
    });

    readonly form: FormGroup<FormScheme<AdmissionSettingForm, 'policies'>> = this.formBuilder.group({
        policies: this.formBuilder.record<AdmissionSettingPolicyRecord>({}),
        historyControl: [AdmissionMonitorHistoryControl.NONE]
    });

    readonly admissionHasChanges$: Observable<boolean> = this.admissionStoreService.admissionHasChanges$;

    readonly admissionHasPoliciesChanges$: Observable<boolean> =
        this.admissionStoreService.admissionHasPoliciesChanges$;

    readonly admissionIsExpertMode$: Observable<boolean> = this.admissionStoreService.admissionIsExpertMode$.pipe(
        tap((isExpertMode) => {
            if (isExpertMode) {
                this.expertModeForm.get('isExpert')?.setValue(true, { onlySelf: true });
            }
        })
    );

    readonly admissionIsOverlayed$: Observable<boolean> = this.admissionStoreService.admissionIsOverlayed$;

    readonly clusters$: Observable<RegisteredCluster[]> = this.clusterStoreService.registeredClusters$;

    readonly activeClusterHost$ = this.apiPathService.host$;

    private configSnapshot?: AdmissionMonitorConfig;
    private readonly config$: Observable<AdmissionMonitorConfig> =
        this.admissionStoreService.admissionMonitorConfig$.pipe(
            tap((config) => {
                this.configSnapshot = config;
                this.setForms(config);
            })
        );

    private readonly hasChanges$ = this.form.valueChanges.pipe(
        startWith(this.form.value),
        /* eslint @typescript-eslint/no-magic-numbers: "off" */
        debounceTime(250),
        distinctUntilChanged(),
        tap(() => {
            const formValues = utils.getFormValues<AdmissionSettingForm>(this.form.controls);
            this.admissionStoreService.checkChanges(admissionConfigUtils.convertSettingFormToMonitorConfig(formValues));
        })
    );

    private readonly toggleExpertMode$ = this.expertModeForm.valueChanges.pipe(
        map((values) => values.isExpert),
        tap((isExpert) => {
            if (isExpert) {
                this.openSwitchExpertModeModal();
            } else {
                this.admissionStoreService.switchExpertMode();
            }
        })
    );

    /* eslint @typescript-eslint/dot-notation: "off" */
    readonly permissions: RolePermissionMap = this.route.snapshot.data['permissions'];

    readonly permissionType = PermissionType;

    readonly permissionName = PermissionName;

    readonly admissionPolicyActionOptions = ADMISSION_POLICY_ACTION;

    readonly admissionSeverityOptions = ADMISSION_SEVERITY;

    readonly admissionHistoryControlOptions = ADMISSION_HISTORY_CONTROL;

    readonly admissionNavigationTabs = ADMISSION_NAVIGATION_TABS;

    readonly alertColors = KbqAlertColors;

    private openPolicySidepanelKey: string | null = null;

    get policiesFormGroup(): FormRecord {
        return this.form?.get('policies') as FormRecord;
    }

    constructor(
        private readonly cdr: ChangeDetectorRef,
        private readonly destroyRef: DestroyRef,
        private readonly formBuilder: FormBuilder,
        private readonly i18nService: I18nService,
        private readonly clusterStoreService: ClusterStoreService,
        private readonly apiPathService: ApiPathService,
        private readonly sharedModalService: SharedModalService,
        private readonly route: ActivatedRoute,
        private readonly router: Router,
        private readonly admissionStoreService: AdmissionStoreService,
        private readonly sidepanelService: KbqSidepanelService
    ) {}

    ngOnInit() {
        this.config$.pipe(takeUntilDestroyed(this.destroyRef)).subscribe();
        this.hasChanges$.pipe(takeUntilDestroyed(this.destroyRef)).subscribe();
        this.toggleExpertMode$.pipe(takeUntilDestroyed(this.destroyRef)).subscribe();
    }

    ngOnDestroy() {
        if (this.openPolicySidepanelKey) {
            this.sidepanelService.closeAll();
        }
    }

    tabChange(path?: string) {
        this.router.navigate([RouterName.ADMISSION, path]);
    }

    openViewPolicySidepanel(key: string, policy: AdmissionSettingPolicyForm) {
        const config: KbqSidepanelConfig<Partial<AdmissionSidepanelPolicyProps>> = {
            position: KbqSidepanelPosition.Right,
            hasBackdrop: false,
            data: {
                name: policy.name,
                description: policy.description,
                yaml: policy.yaml
            }
        };

        if (this.openPolicySidepanelKey) {
            this.sidepanelService.closeAll();
        }

        if (this.openPolicySidepanelKey !== key) {
            const sidepanel = this.sidepanelService.open(AdmissionFeatureSidepanelPolicyComponent, config);
            sidepanel
                .afterOpened()
                .pipe(take(1))
                .subscribe(() => {
                    this.openPolicySidepanelKey = key;
                });
            sidepanel
                .afterClosed()
                .pipe(take(1))
                .subscribe(() => {
                    this.openPolicySidepanelKey = null;
                });
        }
    }

    openCreatePolicySidepanel() {
        const config: KbqSidepanelConfig<Partial<AdmissionSidepanelPolicyFormProps>> = {
            position: KbqSidepanelPosition.Right,
            hasBackdrop: true,
            data: {}
        };

        if (this.openPolicySidepanelKey) {
            this.sidepanelService.closeAll();
        }

        this.sidepanelService
            .open(AdmissionFeatureSidepanelPolicyFormComponent, config)
            .afterClosed()
            .pipe(take(1), filter(utils.isDefined))
            .subscribe((form: AdmissionSettingPolicyForm) => {
                this.policiesFormGroup.addControl(
                    utils.generateUuid('zz'),
                    this.getPolicyFormGroup({
                        isEnabled: false,
                        name: form.name,
                        description: form.description,
                        yaml: form.yaml,
                        action: AdmissionMonitorPolicyAction.AUDIT,
                        severity: AdmissionMonitorSeverity.MEDIUM
                    })
                );
            });
    }

    openEditPolicySidepanel(policy: AdmissionSettingPolicyForm, key: string) {
        const config: KbqSidepanelConfig<Partial<AdmissionSidepanelPolicyFormProps>> = {
            position: KbqSidepanelPosition.Right,
            hasBackdrop: true,
            data: {
                isEdit: true,
                name: policy.name,
                description: policy.description,
                yaml: policy.yaml
            }
        };

        if (this.openPolicySidepanelKey) {
            this.sidepanelService.closeAll();
        }

        this.sidepanelService
            .open(AdmissionFeatureSidepanelPolicyFormComponent, config)
            .afterClosed()
            .pipe(take(1), filter(utils.isDefined))
            .subscribe((form: AdmissionSettingPolicyForm) => {
                this.policiesFormGroup.setControl(
                    key,
                    this.getPolicyFormGroup({
                        isEnabled: policy.isEnabled,
                        name: form.name,
                        description: form.description,
                        yaml: form.yaml,
                        action: policy.action,
                        severity: policy.severity
                    })
                );
                // @todo: refactor this part to remove cdr service
                this.cdr.markForCheck();
            });
    }

    openDeletePolicyModal(key: string) {
        this.sharedModalService.delete({
            title: this.i18nService.translate('Admission.PolicyDeleteModal.Content.Title'),
            content: this.i18nService.translate('Admission.PolicyDeleteModal.Content.Text'),
            confirmText: this.i18nService.translate('Admission.PolicyDeleteModal.Button.Confirm'),
            cancelText: this.i18nService.translate('Admission.PolicyDeleteModal.Button.Cancel'),
            confirmHandler: () => {
                this.policiesFormGroup.removeControl(key);
                // @todo: refactor this part to remove cdr service
                this.cdr.markForCheck();
            }
        });
    }

    create() {
        const formValues = utils.getFormValues<AdmissionSettingForm>(this.form.controls);
        this.admissionStoreService.createConfig(admissionConfigUtils.convertSettingFormToMonitorConfig(formValues));

        this.form.updateValueAndValidity();
    }

    cancel() {
        this.admissionStoreService.hideOverlay();
    }

    reset() {
        if (this.configSnapshot) {
            this.setForms(this.configSnapshot);
        }
    }

    switchCluster(id: string) {
        this.clusterStoreService.switchCluster(id);
    }

    private openSwitchExpertModeModal() {
        this.sharedModalService.delete({
            title: this.i18nService.translate('Admission.SwitchModeModal.Content.Title'),
            content: this.i18nService.translate('Admission.SwitchModeModal.Content.Text'),
            confirmText: this.i18nService.translate('Admission.SwitchModeModal.Button.Confirm'),
            cancelText: this.i18nService.translate('Admission.SwitchModeModal.Button.Cancel'),
            confirmHandler: () => {
                this.admissionStoreService.switchExpertMode();
            },
            cancelHandler: () => {
                this.expertModeForm.get('isExpert')?.setValue(false, { onlySelf: true });
            }
        });
    }

    private getPolicyFormGroup(policy: AdmissionSettingPolicyForm): FormGroup<FormScheme<AdmissionSettingPolicyForm>> {
        return this.formBuilder.group({
            isEnabled: [policy.isEnabled],
            name: [policy.name],
            description: [policy.description],
            yaml: [policy.yaml],
            action: [policy.action],
            severity: [policy.severity]
        });
    }

    private setForms(config: AdmissionMonitorConfig) {
        if (config.history_control !== undefined) {
            this.form.patchValue({
                historyControl: config.history_control
            });
        }

        this.setPoliciesForm(config.policies);

        if (!this.permissions[PermissionName.SYSTEM].has(PermissionType.UPDATE)) {
            this.form.get('historyControl')?.disable({ onlySelf: true });
        }
    }

    private setPoliciesForm(policies: AdmissionMonitorPolicies) {
        Object.keys(this.policiesFormGroup.controls).forEach((key) => {
            this.policiesFormGroup.removeControl(key);
        });

        Object.keys(policies).forEach((key) => {
            const policyValue: AdmissionMonitorPolicy = policies[key];

            this.policiesFormGroup.addControl(
                key,
                this.getPolicyFormGroup({
                    isEnabled: policyValue.enabled || false,
                    name: policyValue.name || '',
                    description: policyValue.description || '',
                    yaml: policyValue.yaml || '',
                    action: policyValue.action || AdmissionMonitorPolicyAction.AUDIT,
                    severity: policyValue.severity || AdmissionMonitorSeverity.MEDIUM
                })
            );
        });
    }
}
