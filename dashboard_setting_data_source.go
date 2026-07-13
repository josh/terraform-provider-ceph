package main

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	dataSourceSchema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/josh/terraform-provider-ceph/internal/cephvalues"
	"github.com/josh/terraform-provider-ceph/internal/restapi"
)

var _ datasource.DataSource = &DashboardSettingDataSource{}

func newDashboardSettingDataSource() datasource.DataSource {
	return &DashboardSettingDataSource{}
}

type DashboardSettingDataSource struct {
	client *restapi.Client
}

type DashboardSettingDataSourceModel struct {
	Name    types.String `tfsdk:"name"`
	Value   types.String `tfsdk:"value"`
	Type    types.String `tfsdk:"type"`
	Default types.String `tfsdk:"default"`
}

func (d *DashboardSettingDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_dashboard_setting"
}

func (d *DashboardSettingDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = dataSourceSchema.Schema{
		MarkdownDescription: "This data source allows you to get the value of a Ceph dashboard setting.",
		Attributes: map[string]dataSourceSchema.Attribute{
			"name": dataSourceSchema.StringAttribute{
				MarkdownDescription: "The name of the dashboard setting",
				Required:            true,
			},
			"value": dataSourceSchema.StringAttribute{
				MarkdownDescription: "The current value of the setting as a string",
				Computed:            true,
			},
			"type": dataSourceSchema.StringAttribute{
				MarkdownDescription: "The native type of the setting",
				Computed:            true,
			},
			"default": dataSourceSchema.StringAttribute{
				MarkdownDescription: "The default value of the setting as a string",
				Computed:            true,
			},
		},
	}
}

func (d *DashboardSettingDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	client, ok := req.ProviderData.(*restapi.Client)

	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			fmt.Sprintf("Expected *restapi.Client, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return
	}

	d.client = client
}

func (d *DashboardSettingDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data DashboardSettingDataSourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	setting, err := d.client.GetDashboardSetting(ctx, data.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to get dashboard setting '%s' from Ceph API: %s", data.Name.ValueString(), err),
		)
		return
	}

	value, err := cephvalues.FormatMgrModuleValue(setting.Value)
	if err != nil {
		resp.Diagnostics.AddError(
			"API Response Error",
			fmt.Sprintf("Unable to format setting value: %s", err),
		)
		return
	}
	defaultValue, err := cephvalues.FormatMgrModuleValue(setting.Default)
	if err != nil {
		resp.Diagnostics.AddError(
			"API Response Error",
			fmt.Sprintf("Unable to format setting default: %s", err),
		)
		return
	}

	data.Value = types.StringValue(value)
	data.Type = types.StringValue(setting.Type)
	data.Default = types.StringValue(defaultValue)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
