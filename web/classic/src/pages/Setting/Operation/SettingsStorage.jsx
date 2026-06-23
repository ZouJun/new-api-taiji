/*
Copyright (C) 2025 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/

import React, { useEffect, useRef, useState } from 'react';
import {
  Button,
  Col,
  Form,
  Row,
  Spin,
  Typography,
} from '@douyinfe/semi-ui';
import { useTranslation } from 'react-i18next';
import {
  API,
  compareObjects,
  showError,
  showSuccess,
  showWarning,
} from '../../../helpers';

const { Text, Title } = Typography;

const backendOptions = [
  { value: 'local', label: '本地' },
  { value: 'azure_blob', label: 'AzureBlob' },
];

const recommendedDefaults = {
  ArchiveEnabled: false,
  ArchiveBackend: 'local',
  ArchiveLocalDir: './data/archive',
  ArchiveSpoolDir: '',
  ArchiveQueueSize: 50000,
  ArchiveWorkerCount: 32,
  ArchiveMaxRequestMB: 128,
  ArchiveMaxResponseMB: 128,
  ArchiveSpoolTTLHours: 24,
  ArchiveSmallPayloadMaxKB: 64,
  ArchiveSegmentMaxMB: 256,
  ArchiveSegmentMaxAgeSeconds: 60,
  ArchiveSegmentMaxRecords: 50000,
  ArchiveSegmentShardCount: 16,
  ArchiveHeaderValueMaxLength: 512,
  ArchiveSamplePercent: 100,
  ArchiveSkipOnHighLoad: true,
  ArchiveMaxCPUPercent: 80,
  ArchiveMaxMemoryPercent: 80,
  ArchiveMinFreeDiskPercent: 15,
  ArchiveMinFreeDiskGB: 10,
  ArchiveLoadCheckIntervalSeconds: 5,
  ArchiveAzureAccountURL: '',
  ArchiveAzureContainer: '',
  ArchiveAzureAccountName: '',
  ArchiveAzureAccountKey: '',
};

const sections = {
  throughput: [
    'ArchiveQueueSize',
    'ArchiveWorkerCount',
    'ArchiveMaxRequestMB',
    'ArchiveMaxResponseMB',
    'ArchiveSpoolTTLHours',
    'ArchiveHeaderValueMaxLength',
    'ArchiveSamplePercent',
  ],
  segment: [
    'ArchiveSmallPayloadMaxKB',
    'ArchiveSegmentMaxMB',
    'ArchiveSegmentMaxAgeSeconds',
    'ArchiveSegmentMaxRecords',
    'ArchiveSegmentShardCount',
  ],
  guard: [
    'ArchiveMaxCPUPercent',
    'ArchiveMaxMemoryPercent',
    'ArchiveMinFreeDiskPercent',
    'ArchiveMinFreeDiskGB',
    'ArchiveLoadCheckIntervalSeconds',
  ],
  azure: [
    'ArchiveAzureAccountURL',
    'ArchiveAzureContainer',
    'ArchiveAzureAccountName',
    'ArchiveAzureAccountKey',
  ],
};

const fieldMeta = {
  ArchiveEnabled: {
    label: '启用存储归档',
    help: 'phase3 原始请求体和响应体归档总开关。关闭时主链路不进入归档流程。',
    recommendation: '默认关闭。只有在审计、回放、争议排查时再开启。',
  },
  ArchiveBackend: {
    label: '存储位置',
    help: '最终归档对象的落点。local 写本地；azure_blob 先本地暂存，再上传 Azure Blob。',
    recommendation: '单机优先 local；需要集中留存时使用 azure_blob。',
  },
  ArchiveLocalDir: {
    label: '归档根目录',
    help: '本地归档对象和默认暂存目录的根路径，应位于持久化磁盘。',
    recommendation: '建议独立 SSD 路径，例如 ./data/archive 或挂载数据盘。',
  },
  ArchiveSpoolDir: {
    label: '暂存目录',
    help: '请求和响应在归档定稿前的临时目录。留空时自动使用 {ArchiveLocalDir}/spool。',
    recommendation: '一般留空即可；需要分盘时再单独配置。',
  },
  ArchiveQueueSize: {
    label: '队列大小',
    help: '后台归档队列可缓存的最大任务数，过大会增加内存占用。',
    recommendation: '8000-15000 RPM 建议 20000-50000。',
  },
  ArchiveWorkerCount: {
    label: 'Worker 数量',
    help: '后台归档 worker 并发数，越高吞吐越大，但磁盘和网络并发也更高。',
    recommendation: 'SSD 机器建议 16-32；共享盘建议 8-16。',
  },
  ArchiveMaxRequestMB: {
    label: '请求上限(MB)',
    help: '超过该大小的原始请求体不归档，避免超大对象拖慢系统。',
    recommendation: '建议 64-128MB。',
  },
  ArchiveMaxResponseMB: {
    label: '响应上限(MB)',
    help: '超过该大小的原始响应体不归档，用于限制极端大响应成本。',
    recommendation: '建议 64-128MB。',
  },
  ArchiveSpoolTTLHours: {
    label: '暂存保留(小时)',
    help: '临时暂存文件的保留时间，用于故障恢复和中断清理。',
    recommendation: '建议 6-24 小时。',
  },
  ArchiveSmallPayloadMaxKB: {
    label: '小文件阈值(KB)',
    help: '不超过该阈值的请求走 segmented 聚合，大于该阈值走 per_request。',
    recommendation: '建议 32-128KB。',
  },
  ArchiveSegmentMaxMB: {
    label: 'Segment 上限(MB)',
    help: '聚合 segment 文件的最大体积，到达后轮转新文件。',
    recommendation: 'SSD 建议 128-256MB。',
  },
  ArchiveSegmentMaxAgeSeconds: {
    label: 'Segment 时长(秒)',
    help: '单个聚合 segment 的最大打开时间，即使未写满也会轮转。',
    recommendation: '建议 30-60 秒。',
  },
  ArchiveSegmentMaxRecords: {
    label: 'Segment 记录数',
    help: '单个 segment 内允许的最大记录数，防止聚合过深。',
    recommendation: '建议 10000-50000。',
  },
  ArchiveSegmentShardCount: {
    label: 'Segment 分片数',
    help: '小文件聚合写入时的分片数量，用于分散并发写压力。',
    recommendation: '建议 8-32。',
  },
  ArchiveHeaderValueMaxLength: {
    label: 'Header 截断长度',
    help: '归档时单个 header 值的最大保留长度，超过部分截断。',
    recommendation: '建议 256-1024，默认 512。',
  },
  ArchiveSamplePercent: {
    label: '采样百分比',
    help: '启用归档后，仍按概率决定是否实际进入归档流程。',
    recommendation: '默认 100；高流量场景可先用 1-10 验证成本。',
  },
  ArchiveSkipOnHighLoad: {
    label: '高负载时跳过归档',
    help: '当 CPU、内存或磁盘空闲低于阈值时，优先保护主链路，直接跳过归档。',
    recommendation: '建议保持开启。',
  },
  ArchiveMaxCPUPercent: {
    label: 'CPU 阈值(%)',
    help: 'CPU 使用率超过该阈值时跳过归档。',
    recommendation: '建议 70-85，默认 80。',
  },
  ArchiveMaxMemoryPercent: {
    label: '内存阈值(%)',
    help: '内存使用率超过该阈值时跳过归档。',
    recommendation: '建议 70-85，默认 80。',
  },
  ArchiveMinFreeDiskPercent: {
    label: '最小空闲磁盘(%)',
    help: '磁盘剩余百分比低于该值时跳过归档。',
    recommendation: '建议 10-20，默认 15。',
  },
  ArchiveMinFreeDiskGB: {
    label: '最小空闲磁盘(GB)',
    help: '磁盘剩余绝对容量低于该值时跳过归档。',
    recommendation: '建议 5-20GB，默认 10GB。',
  },
  ArchiveLoadCheckIntervalSeconds: {
    label: '负载检查间隔(秒)',
    help: '系统负载采样刷新间隔。越短越及时，但也会增加少量采样开销。',
    recommendation: '建议 3-10 秒，默认 5 秒。',
  },
  ArchiveAzureAccountURL: {
    label: 'Azure Account URL',
    help: 'Azure Blob 存储账号 URL，仅在后端选择 azure_blob 时需要。',
    recommendation: '例如 https://<account>.blob.core.windows.net。',
  },
  ArchiveAzureContainer: {
    label: 'Azure Container',
    help: 'Azure Blob 的目标容器名。',
    recommendation: '建议使用专用容器。',
  },
  ArchiveAzureAccountName: {
    label: 'Azure Account Name',
    help: 'Azure 存储账号名称，用于签名认证。',
    recommendation: '仅在所用认证方式需要时填写。',
  },
  ArchiveAzureAccountKey: {
    label: 'Azure Account Key',
    help: 'Azure 存储账号密钥，属于敏感配置。',
    recommendation: '建议配合独立账号和最小权限使用。',
  },
};

function renderLabel(t, key) {
  const meta = fieldMeta[key];
  return (
    <span style={{ display: 'inline-flex', flexWrap: 'wrap', gap: 8, alignItems: 'center', lineHeight: 1.5 }}>
      <span>{t(meta.label)}</span>
      <Text type='tertiary' size='small'>
        {`${t('建议配置')}：${t(meta.recommendation)}`}
      </Text>
    </span>
  );
}

function getPlaceholderValue(key) {
  const recommended = recommendedDefaults[key];
  if (typeof recommended === 'number') {
    return `建议配置：${recommended}`;
  }
  if (typeof recommended === 'string' && recommended !== '') {
    return `建议配置：${recommended}`;
  }

  switch (key) {
    case 'ArchiveSpoolDir':
      return '建议配置：{archive_root}/spool';
    case 'ArchiveAzureAccountURL':
      return '建议配置：https://<account>.blob.core.windows.net';
    case 'ArchiveAzureContainer':
      return '建议配置：archive';
    case 'ArchiveAzureAccountName':
      return '建议配置：storage account name';
    case 'ArchiveAzureAccountKey':
      return '建议配置：使用 SAS URL 时留空';
    default:
      return '';
  }
}

function withRecommendedDefaults(values) {
  const next = { ...values };
  Object.keys(recommendedDefaults).forEach((key) => {
    if (key === 'ArchiveEnabled') return;
    const current = next[key];
    const recommended = recommendedDefaults[key];

    if (typeof recommended === 'number') {
      if (typeof current !== 'number' || current <= 0) {
        next[key] = recommended;
      }
      return;
    }

    if (typeof recommended === 'string' && String(current ?? '').trim() === '') {
      next[key] = recommended;
    }
  });
  return next;
}

function renderNumberField(t, key, updateValue) {
  return (
    <Col key={key} xs={24} sm={12} md={8}>
      <Form.InputNumber
        field={key}
        label={renderLabel(t, key)}
        placeholder={getPlaceholderValue(key)}
        step={1}
        min={0}
        onChange={(value) =>
          updateValue(
            key,
            Number.isNaN(parseInt(value, 10)) ? 0 : parseInt(value, 10)
          )
        }
      />
    </Col>
  );
}

export default function SettingsStorage(props) {
  const { t } = useTranslation();
  const refForm = useRef();
  const [loading, setLoading] = useState(false);
  const [inputs, setInputs] = useState(structuredClone(recommendedDefaults));
  const [snapshot, setSnapshot] = useState(structuredClone(recommendedDefaults));

  const updateValue = (key, value) => {
    setInputs((prev) => ({
      ...prev,
      [key]: value,
    }));
  };

  function onSubmit() {
    const updateArray = compareObjects(inputs, snapshot);
    if (!updateArray.length) {
      showWarning(t('你似乎并没有修改什么'));
      return;
    }

    const requestBody = {
      options: updateArray.map((item) => ({
        key: item.key,
        value:
          typeof inputs[item.key] === 'boolean'
            ? String(inputs[item.key])
            : String(inputs[item.key] ?? '').trim(),
      })),
    };

    setLoading(true);
    API.put('/api/option/bulk', requestBody)
      .then((res) => {
        if (!res.data?.success) {
          throw new Error(res.data?.message || t('保存失败，请重试'));
        }
        showSuccess(t('保存成功'));
        props.refresh();
      })
      .catch((error) => {
        showError(error?.response?.data?.message || t('保存失败，请重试'));
      })
      .finally(() => {
        setLoading(false);
      });
  }

  function onResetDefaults() {
    const next = structuredClone(recommendedDefaults);
    setInputs(next);
    refForm.current?.setValues(next);
  }

  useEffect(() => {
    const currentInputs = {};
    Object.keys(recommendedDefaults).forEach((key) => {
      currentInputs[key] = props.options?.[key] ?? recommendedDefaults[key];
    });
    setInputs(currentInputs);
    setSnapshot(structuredClone(currentInputs));
    refForm.current?.setValues(currentInputs);
  }, [props.options]);

  return (
    <Spin spinning={loading}>
      <Form
        values={inputs}
        getFormApi={(formApi) => {
          refForm.current = formApi;
        }}
        style={{ marginBottom: 15 }}
      >
        <Form.Section text={t('存储设置')}>
          <Row gutter={16}>
            <Col span={24}>
              <Text type='secondary'>
                {t('修改后实时生效，用于 phase3 原始请求/响应归档流程。')}
              </Text>
            </Col>
          </Row>

          <div style={{ marginTop: 16, padding: 16, border: '1px solid var(--semi-color-border)', borderRadius: 12 }}>
            <Title heading={6}>{t('归档总开关')}</Title>
            <Row gutter={16}>
              <Col xs={24} sm={12} md={8}>
                <Form.Switch
                  field='ArchiveEnabled'
                  label={renderLabel(t, 'ArchiveEnabled')}
                  checkedText='｜'
                  uncheckedText='〇'
                  onChange={(value) => {
                    updateValue('ArchiveEnabled', value);
                    if (value) {
                      const next = withRecommendedDefaults({
                        ...inputs,
                        ArchiveEnabled: true,
                      });
                      setInputs(next);
                      refForm.current?.setValues(next);
                    }
                  }}
                />
              </Col>
              <Col xs={24} sm={12} md={8}>
                <Form.Switch
                  field='ArchiveSkipOnHighLoad'
                  label={renderLabel(t, 'ArchiveSkipOnHighLoad')}
                  checkedText='｜'
                  uncheckedText='〇'
                  onChange={(value) => updateValue('ArchiveSkipOnHighLoad', value)}
                />
              </Col>
              <Col xs={24} sm={12} md={8}>
                <Form.Select
                  field='ArchiveBackend'
                  label={renderLabel(t, 'ArchiveBackend')}
                  optionList={backendOptions}
                  onChange={(value) => updateValue('ArchiveBackend', value)}
                />
              </Col>
            </Row>
          </div>

          {!inputs.ArchiveEnabled ? (
            <div
              style={{
                marginTop: 16,
                paddingTop: 16,
                borderTop: '1px solid var(--semi-color-border)',
              }}
            >
              <Text type='secondary'>
                {t('启用存储归档后，才展示其它策略参数，避免误改未生效配置。')}
              </Text>
            </div>
          ) : (
            <>
              <div
                style={{
                  marginTop: 16,
                  paddingTop: 16,
                  borderTop: '1px solid var(--semi-color-border)',
                }}
              >
                <Title heading={6}>{t('本地路径')}</Title>
                <Row gutter={16}>
                  {['ArchiveLocalDir', 'ArchiveSpoolDir'].map((key) => (
                    <Col key={key} xs={24} sm={12}>
                      <Form.Input
                        field={key}
                        label={renderLabel(t, key)}
                        placeholder={getPlaceholderValue(key)}
                        onChange={(value) => updateValue(key, value)}
                      />
                    </Col>
                  ))}
                </Row>
              </div>

              <div
                style={{
                  marginTop: 16,
                  paddingTop: 16,
                  borderTop: '1px solid var(--semi-color-border)',
                }}
              >
                <Title heading={6}>{t('吞吐与对象限制')}</Title>
                <Row gutter={16}>
                  {sections.throughput.map((key) => renderNumberField(t, key, updateValue))}
                </Row>
              </div>

              <div
                style={{
                  marginTop: 16,
                  paddingTop: 16,
                  borderTop: '1px solid var(--semi-color-border)',
                }}
              >
                <Title heading={6}>{t('Segment 聚合')}</Title>
                <Row gutter={16}>
                  {sections.segment.map((key) => renderNumberField(t, key, updateValue))}
                </Row>
              </div>

              <div
                style={{
                  marginTop: 16,
                  paddingTop: 16,
                  borderTop: '1px solid var(--semi-color-border)',
                }}
              >
                <Title heading={6}>{t('负载保护')}</Title>
                <Row gutter={16}>
                  {sections.guard.map((key) => renderNumberField(t, key, updateValue))}
                </Row>
              </div>

              {inputs.ArchiveBackend === 'azure_blob' ? (
                <div
                  style={{
                    marginTop: 16,
                    paddingTop: 16,
                    borderTop: '1px solid var(--semi-color-border)',
                  }}
                >
                  <Title heading={6}>{t('Azure Blob 连接')}</Title>
                  <Row gutter={16}>
                    {sections.azure.map((key) => (
                      <Col key={key} xs={24} sm={12}>
                        <Form.Input
                          field={key}
                          label={renderLabel(t, key)}
                          type={key === 'ArchiveAzureAccountKey' ? 'password' : 'text'}
                          placeholder={getPlaceholderValue(key)}
                          onChange={(value) => updateValue(key, value)}
                        />
                      </Col>
                    ))}
                  </Row>
                </div>
              ) : null}
            </>
          )}

          <div style={{ display: 'flex', justifyContent: 'flex-start', gap: 12, marginTop: 20 }}>
            <Button onClick={onResetDefaults}>{t('重置为默认')}</Button>
            <Button theme='solid' onClick={onSubmit}>
              {t('保存设置')}
            </Button>
          </div>
        </Form.Section>
      </Form>
    </Spin>
  );
}
