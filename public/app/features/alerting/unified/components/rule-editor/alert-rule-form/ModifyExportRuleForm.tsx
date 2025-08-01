import { memo, useCallback, useEffect, useMemo, useState } from 'react';
import { FormProvider, useForm } from 'react-hook-form';
import { useAsync } from 'react-use';

import { Trans, t } from '@grafana/i18n';
import { Button, LinkButton, LoadingPlaceholder, Stack } from '@grafana/ui';
import { useAppNotification } from 'app/core/copy/appNotification';

import {
  PostableRulerRuleGroupDTO,
  RulerRuleDTO,
  RulerRuleGroupDTO,
} from '../../../../../../types/unified-alerting-dto';
import { alertRuleApi } from '../../../api/alertRuleApi';
import { fetchRulerRulesGroup } from '../../../api/ruler';
import { useDataSourceFeatures } from '../../../hooks/useCombinedRule';
import { useReturnTo } from '../../../hooks/useReturnTo';
import { DEFAULT_GROUP_EVALUATION_INTERVAL, getDefaultFormValues } from '../../../rule-editor/formDefaults';
import { RuleFormType, RuleFormValues } from '../../../types/rule-form';
import { GRAFANA_RULES_SOURCE_NAME } from '../../../utils/datasource';
import { formValuesToRulerGrafanaRuleDTO, getDefaultQueries } from '../../../utils/rule-form';
import { rulerRuleType } from '../../../utils/rules';
import { FileExportPreview } from '../../export/FileExportPreview';
import { GrafanaExportDrawer } from '../../export/GrafanaExportDrawer';
import { ExportFormats, HclExportProvider, allGrafanaExportProviders } from '../../export/providers';
import { AlertRuleNameAndMetric } from '../AlertRuleNameInput';
import AnnotationsStep from '../AnnotationsStep';
import { GrafanaEvaluationBehaviorStep } from '../GrafanaEvaluationBehavior';
import { GrafanaFolderAndLabelsStep } from '../GrafanaFolderAndLabelsStep';
import { NotificationsStep } from '../NotificationsStep';
import { QueryAndExpressionsStep } from '../query-and-alert-condition/QueryAndExpressionsStep';

interface ModifyExportRuleFormProps {
  alertUid?: string;
  ruleForm?: RuleFormValues;
}

export function ModifyExportRuleForm({ ruleForm, alertUid }: ModifyExportRuleFormProps) {
  const defaultValuesForNewRule: RuleFormValues = useMemo(() => {
    const defaultRuleType = RuleFormType.grafana;

    return {
      ...getDefaultFormValues(),
      condition: 'C',
      queries: getDefaultQueries(false),
      type: defaultRuleType,
      evaluateEvery: DEFAULT_GROUP_EVALUATION_INTERVAL,
    };
  }, []);

  const formAPI = useForm<RuleFormValues>({
    mode: 'onSubmit',
    defaultValues: ruleForm ?? defaultValuesForNewRule,
    shouldFocusError: true,
  });

  const existing = Boolean(ruleForm);
  const notifyApp = useAppNotification();
  const { returnTo } = useReturnTo('/alerting/list');

  const [exportData, setExportData] = useState<RuleFormValues | undefined>(undefined);
  const [conditionErrorMsg, setConditionErrorMsg] = useState('');

  const onInvalid = (): void => {
    notifyApp.error('There are errors in the form. Please correct them and try again!');
  };

  const checkAlertCondition = (msg = '') => {
    setConditionErrorMsg(msg);
  };

  const submit = (exportData: RuleFormValues | undefined) => {
    if (conditionErrorMsg !== '') {
      notifyApp.error(conditionErrorMsg);
      return;
    }
    setExportData(exportData);
  };

  const onClose = useCallback(() => {
    setExportData(undefined);
  }, [setExportData]);

  return (
    <FormProvider {...formAPI}>
      <Stack direction="column">
        <form onSubmit={(e) => e.preventDefault()}>
          <div>
            <Stack direction="column" gap={3}>
              {/* Step 1 */}
              <AlertRuleNameAndMetric />
              {/* Step 2 */}
              <QueryAndExpressionsStep editingExistingRule={existing} onDataChange={checkAlertCondition} mode="draft" />
              {/* Step 3-4-5 */}
              <GrafanaFolderAndLabelsStep />

              {/* Step 4 & 5 */}
              <GrafanaEvaluationBehaviorStep existing={Boolean(existing)} enableProvisionedGroups={true} />
              {/* Notifications step*/}
              <NotificationsStep alertUid={alertUid} />
              {/* Annotations only for cloud and Grafana */}
              <AnnotationsStep />
            </Stack>
          </div>
        </form>
        {exportData && <GrafanaRuleDesignExporter exportValues={exportData} onClose={onClose} uid={alertUid} />}
        <Stack direction="row">
          <Button key="export-rule" onClick={formAPI.handleSubmit((formValues) => submit(formValues), onInvalid)}>
            <Trans i18nKey="alerting.modify-export-rule-form.action-buttons.export">Export</Trans>
          </Button>
          <LinkButton href={returnTo} key="cancel" variant="secondary" onClick={() => submit(undefined)}>
            <Trans i18nKey="alerting.common.cancel">Cancel</Trans>
          </LinkButton>
        </Stack>
      </Stack>
    </FormProvider>
  );
}

const useGetGroup = (nameSpaceUID: string, group: string) => {
  const { dsFeatures } = useDataSourceFeatures(GRAFANA_RULES_SOURCE_NAME);

  const rulerConfig = dsFeatures?.rulerConfig;

  const targetGroup = useAsync(async () => {
    return rulerConfig ? await fetchRulerRulesGroup(rulerConfig, nameSpaceUID, group) : undefined;
  }, [rulerConfig, nameSpaceUID, group]);

  return targetGroup;
};

interface GrafanaRuleDesignExportPreviewProps {
  exportFormat: ExportFormats;
  onClose: () => void;
  exportValues: RuleFormValues;
  uid?: string;
}
export const getPayloadToExport = (
  formValues: RuleFormValues,
  existingGroup: RulerRuleGroupDTO<RulerRuleDTO> | null | undefined,
  ruleUid?: string
): PostableRulerRuleGroupDTO => {
  const grafanaRuleDto = formValuesToRulerGrafanaRuleDTO(formValues);

  const updatedRule = { ...grafanaRuleDto, grafana_alert: { ...grafanaRuleDto.grafana_alert, uid: ruleUid } };
  if (existingGroup?.rules) {
    // we have to update the rule in the group in the same position if it exists, otherwise we have to add it at the end
    let alreadyExistsInGroup = false;
    const updatedRules = existingGroup.rules.map((rule: RulerRuleDTO) => {
      if (rulerRuleType.grafana.rule(rule) && rule.grafana_alert.uid === ruleUid) {
        alreadyExistsInGroup = true;
        return updatedRule;
      } else {
        return rule;
      }
    });
    if (!alreadyExistsInGroup) {
      // we have to add the updated rule at the end of the group
      updatedRules.push(updatedRule);
    }
    return {
      ...existingGroup,
      rules: updatedRules,
    };
  } else {
    // we have to create a new group with the updated rule
    return {
      name: existingGroup?.name ?? formValues.group,
      rules: [updatedRule],
    };
  }
};

const useGetPayloadToExport = (values: RuleFormValues, ruleUid?: string) => {
  const rulerGroupDto = useGetGroup(values.folder?.uid ?? '', values.group);
  const payload: PostableRulerRuleGroupDTO = useMemo(() => {
    return getPayloadToExport(values, rulerGroupDto?.value, ruleUid);
  }, [ruleUid, rulerGroupDto, values]);
  return { payload, loadingGroup: rulerGroupDto.loading };
};

const GrafanaRuleDesignExportPreview = ({
  exportFormat,
  exportValues,
  onClose,
  uid,
}: GrafanaRuleDesignExportPreviewProps) => {
  const [getExport, exportData] = alertRuleApi.endpoints.exportModifiedRuleGroup.useMutation();
  const { loadingGroup, payload } = useGetPayloadToExport(exportValues, uid);

  const nameSpaceUID = exportValues.folder?.uid ?? '';

  useEffect(() => {
    !loadingGroup && payload.name && getExport({ payload, format: exportFormat, nameSpaceUID });
  }, [nameSpaceUID, exportFormat, payload, getExport, loadingGroup]);

  if (exportData.isLoading) {
    return <LoadingPlaceholder text={t('alerting.grafana-rule-design-export-preview.text-loading', 'Loading....')} />;
  }

  const downloadFileName = `modify-export-${payload.name}-${uid}-${new Date().getTime()}`;

  return (
    <FileExportPreview
      format={exportFormat}
      textDefinition={exportData.data ?? ''}
      downloadFileName={downloadFileName}
      onClose={onClose}
    />
  );
};

interface GrafanaRuleDesignExporterProps {
  onClose: () => void;
  exportValues: RuleFormValues;
  uid?: string;
}

export const GrafanaRuleDesignExporter = memo(({ onClose, exportValues, uid }: GrafanaRuleDesignExporterProps) => {
  const exportingNewRule = !uid;
  const initialTab = exportingNewRule ? 'hcl' : 'yaml';
  const [activeTab, setActiveTab] = useState<ExportFormats>(initialTab);

  const formatProviders = exportingNewRule ? [HclExportProvider] : Object.values(allGrafanaExportProviders);

  return (
    <GrafanaExportDrawer
      title={t('alerting.grafana-rule-design-exporter.title-export-group', 'Export Group')}
      activeTab={activeTab}
      onTabChange={setActiveTab}
      onClose={onClose}
      formatProviders={formatProviders}
    >
      <GrafanaRuleDesignExportPreview
        exportFormat={activeTab}
        onClose={onClose}
        exportValues={exportValues}
        uid={uid}
      />
    </GrafanaExportDrawer>
  );
});

GrafanaRuleDesignExporter.displayName = 'GrafanaRuleDesignExporter';
