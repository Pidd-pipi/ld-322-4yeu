import { useState } from 'react';
import {
  Button,
  Card,
  Form,
  Input,
  Modal,
  Popconfirm,
  Select,
  Space,
  Switch,
  Table,
  Tag,
  Typography,
  message,
} from 'antd';
import type { ColumnsType } from 'antd/es/table';
import { PlusOutlined } from '@ant-design/icons';
import type { AutomationRule, AutomationRuleRequest, DeviceAction, Greenhouse } from '../../types/domain';
import { createAutomationRule, deleteAutomationRule, setAutomationRuleEnabled } from '../../api/automation';

const { Text } = Typography;

interface Props {
  greenhouse: Greenhouse;
  rules: AutomationRule[];
  onUpdated: () => void;
}

const actionText = (action?: string) => action === 'on' ? '开启' : action === 'off' ? '关闭' : '—';

export default function AutomationRulePanel({ greenhouse, rules, onUpdated }: Props) {
  const [open, setOpen] = useState(false);
  const [saving, setSaving] = useState(false);
  const [busyId, setBusyId] = useState<number>();
  const [form] = Form.useForm<AutomationRuleRequest>();

  const openModal = () => {
    form.setFieldsValue({
      greenhouseId: greenhouse.id,
      abnormalAction: 'on',
      normalAction: 'off',
      enabled: true,
      sensorId: greenhouse.sensors[0]?.id,
      deviceId: greenhouse.devices[0]?.id,
    });
    setOpen(true);
  };

  const updateOppositeAction = (value: DeviceAction) => {
    form.setFieldValue('normalAction', value === 'on' ? 'off' : 'on');
  };

  const submit = async () => {
    const values = await form.validateFields();
    if (values.abnormalAction === values.normalAction) {
      message.warning('异常动作和恢复动作必须相反');
      return;
    }
    try {
      setSaving(true);
      await createAutomationRule({ ...values, greenhouseId: greenhouse.id });
      message.success('联动规则已创建');
      setOpen(false);
      onUpdated();
    } catch {
      message.error('规则创建失败，请检查配置');
    } finally {
      setSaving(false);
    }
  };

  const toggleRule = async (rule: AutomationRule, enabled: boolean) => {
    try {
      setBusyId(rule.id);
      await setAutomationRuleEnabled(rule.id, enabled);
      message.success(`规则已${enabled ? '启用' : '停用'}`);
      onUpdated();
    } catch {
      message.error('规则状态更新失败');
    } finally {
      setBusyId(undefined);
    }
  };

  const removeRule = async (rule: AutomationRule) => {
    try {
      setBusyId(rule.id);
      await deleteAutomationRule(rule.id);
      message.success('规则已移除');
      onUpdated();
    } catch {
      message.error('规则移除失败');
    } finally {
      setBusyId(undefined);
    }
  };

  const columns: ColumnsType<AutomationRule> = [
    {
      title: '规则 / 传感器',
      dataIndex: 'name',
      render: (_, rule) => (
        <Space direction="vertical" size={0}>
          <Text strong>{rule.name}</Text>
          <Text type="secondary">{rule.sensor?.name ?? `传感器 #${rule.sensorId}`}</Text>
        </Space>
      ),
    },
    {
      title: '控制设备',
      render: (_, rule) => (
        <Space direction="vertical" size={0}>
          <Text>{rule.device?.name ?? `设备 #${rule.deviceId}`}</Text>
          <Text type="secondary">超限{actionText(rule.abnormalAction)} · 恢复{actionText(rule.normalAction)}</Text>
        </Space>
      ),
    },
    {
      title: '当前状态',
      width: 110,
      render: (_, rule) => (
        <Space direction="vertical" size={2}>
          <Tag color={rule.lastState === 'abnormal' ? 'error' : 'success'}>
            {rule.lastState === 'abnormal' ? '异常中' : '正常'}
          </Tag>
          {!rule.enabled && <Tag>已停用</Tag>}
        </Space>
      ),
    },
    {
      title: '最近一次执行结果',
      render: (_, rule) => {
        if (!rule.lastExecutedAt) return <Text type="secondary">尚未执行</Text>;
        return (
          <Space direction="vertical" size={0}>
            <Space size={6}>
              <Tag color={rule.lastExecutionStatus === 'success' ? 'success' : 'error'}>
                {rule.lastExecutionStatus === 'success' ? '成功' : '失败'}
              </Tag>
              <Text type="secondary">{new Date(rule.lastExecutedAt).toLocaleString('zh-CN')}</Text>
            </Space>
            <Text type={rule.lastExecutionStatus === 'success' ? undefined : 'danger'}>{rule.lastExecutionMessage}</Text>
          </Space>
        );
      },
    },
    {
      title: '启停',
      width: 80,
      render: (_, rule) => (
        <Switch
          checked={rule.enabled}
          loading={busyId === rule.id}
          onChange={(enabled) => void toggleRule(rule, enabled)}
        />
      ),
    },
    {
      title: '操作',
      width: 88,
      render: (_, rule) => (
        <Popconfirm title="确定移除这条联动规则？" onConfirm={() => void removeRule(rule)}>
          <Button type="link" danger loading={busyId === rule.id}>移除</Button>
        </Popconfirm>
      ),
    },
  ];

  return (
    <Card
      title="阈值联动规则"
      extra={<Space><Button size="small" onClick={onUpdated}>刷新</Button><Button type="primary" size="small" icon={<PlusOutlined />} onClick={openModal}>新建规则</Button></Space>}
    >
      <Table rowKey="id" size="small" columns={columns} dataSource={rules} pagination={false} scroll={{ x: 920 }} locale={{ emptyText: '暂无联动规则' }} />
      <Modal title="新建阈值联动规则" open={open} onCancel={() => setOpen(false)} onOk={() => void submit()} confirmLoading={saving} destroyOnClose>
        <Form form={form} layout="vertical" preserve={false}>
          <Form.Item name="name" label="规则名称" rules={[{ required: true, message: '请输入规则名称' }, { min: 2, message: '名称至少 2 个字符' }]}>
            <Input placeholder="例如：高温开启循环风机" maxLength={100} />
          </Form.Item>
          <Form.Item name="sensorId" label="监测传感器" rules={[{ required: true, message: '请选择传感器' }]}>
            <Select options={greenhouse.sensors.map((sensor) => ({
              value: sensor.id,
              label: `${sensor.name}（${sensor.threshold?.minValue ?? '-'} ~ ${sensor.threshold?.maxValue ?? '-'}${sensor.unit}）`,
            }))} />
          </Form.Item>
          <Form.Item name="deviceId" label="控制设备" rules={[{ required: true, message: '请选择设备' }]}>
            <Select options={greenhouse.devices.map((device) => ({ value: device.id, label: device.name }))} />
          </Form.Item>
          <Form.Item name="abnormalAction" label="读数超出阈值时" rules={[{ required: true }]}>
            <Select onChange={updateOppositeAction} options={[{ value: 'on', label: '开启设备' }, { value: 'off', label: '关闭设备' }]} />
          </Form.Item>
          <Form.Item name="normalAction" label="读数恢复到正常范围时" rules={[{ required: true }]} tooltip="恢复动作固定为异常动作的相反动作">
            <Select disabled options={[{ value: 'on', label: '开启设备' }, { value: 'off', label: '关闭设备' }]} />
          </Form.Item>
          <Form.Item name="enabled" label="创建后立即启用" valuePropName="checked">
            <Switch />
          </Form.Item>
        </Form>
      </Modal>
    </Card>
  );
}
