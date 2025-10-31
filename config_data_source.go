package main

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	dataSourceSchema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSource = &ConfigDataSource{}

func newConfigDataSource() datasource.DataSource {
	return &ConfigDataSource{}
}

type ConfigDataSource struct {
	client *CephAPIClient
}

type ConfigDataSourceModel struct {
	Section types.String `tfsdk:"section"`
	Configs types.List   `tfsdk:"configs"`
}

type ConfigItem struct {
	Section            types.String `tfsdk:"section"`
	Name               types.String `tfsdk:"name"`
	Value              types.String `tfsdk:"value"`
	Level              types.String `tfsdk:"level"`
	CanUpdateAtRuntime types.Bool   `tfsdk:"can_update_at_runtime"`
}

func (d *ConfigDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_config"
}

func (d *ConfigDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = dataSourceSchema.Schema{
		MarkdownDescription: "This data source returns all explicitly set cluster configurations (equivalent to `ceph config dump`).",
		Attributes: map[string]dataSourceSchema.Attribute{
			"section": dataSourceSchema.StringAttribute{
				MarkdownDescription: "Optional filter to only return configs for a specific section (e.g., 'global', 'mon', 'osd')",
				Optional:            true,
			},
			"configs": dataSourceSchema.ListNestedAttribute{
				MarkdownDescription: "List of explicitly set configuration values",
				Computed:            true,
				NestedObject: dataSourceSchema.NestedAttributeObject{
					Attributes: map[string]dataSourceSchema.Attribute{
						"section": dataSourceSchema.StringAttribute{
							MarkdownDescription: "The configuration section (e.g., 'global', 'mon', 'mgr')",
							Computed:            true,
						},
						"name": dataSourceSchema.StringAttribute{
							MarkdownDescription: "The configuration option name",
							Computed:            true,
						},
						"value": dataSourceSchema.StringAttribute{
							MarkdownDescription: "The configuration value",
							Computed:            true,
						},
						"level": dataSourceSchema.StringAttribute{
							MarkdownDescription: "The configuration level (e.g., 'basic', 'advanced', 'dev')",
							Computed:            true,
						},
						"can_update_at_runtime": dataSourceSchema.BoolAttribute{
							MarkdownDescription: "Whether the configuration can be updated at runtime",
							Computed:            true,
						},
					},
				},
			},
		},
	}
}

func (d *ConfigDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *ConfigDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data ConfigDataSourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	configs, err := d.client.ClusterListConf(ctx)
	if err != nil {
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to list cluster configuration from Ceph API: %s", err),
		)
		return
	}

	filterSection := ""
	if !data.Section.IsNull() {
		filterSection = data.Section.ValueString()
	}

	configItems := []ConfigItem{}
	for _, config := range configs {
		if len(config.Value) > 0 {
			for _, v := range config.Value {
				if filterSection == "" || v.Section == filterSection {
					configItems = append(configItems, ConfigItem{
						Section:            types.StringValue(v.Section),
						Name:               types.StringValue(config.Name),
						Value:              types.StringValue(v.Value),
						Level:              types.StringValue(config.Level),
						CanUpdateAtRuntime: types.BoolValue(config.CanUpdateAtRuntime),
					})
				}
			}
		}
	}

	configsValue, diags := types.ListValueFrom(ctx, types.ObjectType{
		AttrTypes: map[string]attr.Type{
			"section":               types.StringType,
			"name":                  types.StringType,
			"value":                 types.StringType,
			"level":                 types.StringType,
			"can_update_at_runtime": types.BoolType,
		},
	}, configItems)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	data.Configs = configsValue

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
