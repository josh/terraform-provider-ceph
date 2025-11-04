package main

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	dataSourceSchema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSource = &MgrModuleConfigDataSource{}

func newMgrModuleConfigDataSource() datasource.DataSource {
	return &MgrModuleConfigDataSource{}
}

type MgrModuleConfigDataSource struct {
	client *CephAPIClient
}

type MgrModuleConfigDataSourceModel struct {
	ModuleName types.String `tfsdk:"module_name"`
	Configs    types.Map    `tfsdk:"configs"`
	ID         types.String `tfsdk:"id"`
}

func (d *MgrModuleConfigDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_mgr_module_config"
}

func (d *MgrModuleConfigDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = dataSourceSchema.Schema{
		MarkdownDescription: "This data source returns the configuration settings for a Ceph MGR module. " +
			"MGR module configs (like `mgr/dashboard/ssl`) are managed separately from cluster configs " +
			"and are accessed via the `/api/mgr/module` endpoint.",
		Attributes: map[string]dataSourceSchema.Attribute{
			"module_name": dataSourceSchema.StringAttribute{
				MarkdownDescription: "The name of the MGR module (e.g., 'dashboard', 'telemetry', 'prometheus')",
				Required:            true,
			},
			"configs": dataSourceSchema.MapAttribute{
				MarkdownDescription: "Map of configuration option names to their current values. " +
					"Values are returned as strings regardless of their underlying type.",
				Computed:    true,
				ElementType: types.StringType,
			},
			"id": dataSourceSchema.StringAttribute{
				MarkdownDescription: "Identifier for this data source (set to module_name)",
				Computed:            true,
			},
		},
	}
}

func (d *MgrModuleConfigDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	client, ok := req.ProviderData.(*CephAPIClient)

	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			fmt.Sprintf("Expected *CephAPIClient, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return
	}

	d.client = client
}

func (d *MgrModuleConfigDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data MgrModuleConfigDataSourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	moduleName := data.ModuleName.ValueString()

	config, err := d.client.MgrGetModuleConfig(ctx, moduleName)
	if err != nil {
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to read MGR module config for '%s' from Ceph API: %s", moduleName, err),
		)
		return
	}

	configMap := make(map[string]string)
	for key, value := range config {
		formattedVal, err := formatMgrModuleConfigValue(value)
		if err != nil {
			resp.Diagnostics.AddError(
				"Configuration Value Formatting Error",
				fmt.Sprintf("Unable to format config value for key '%s': %s", key, err),
			)
			return
		}
		configMap[key] = formattedVal
	}

	configsValue, diags := types.MapValueFrom(ctx, types.StringType, configMap)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	data.Configs = configsValue
	data.ID = types.StringValue(moduleName)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
