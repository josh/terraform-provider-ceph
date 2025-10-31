package main

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	dataSourceSchema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSource = &ConfigValueDataSource{}

func newConfigValueDataSource() datasource.DataSource {
	return &ConfigValueDataSource{}
}

type ConfigValueDataSource struct {
	client *CephAPIClient
}

type ConfigValueDataSourceModel struct {
	Name    types.String `tfsdk:"name"`
	Section types.String `tfsdk:"section"`
	Value   types.String `tfsdk:"value"`
}

func (d *ConfigValueDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_config_value"
}

func (d *ConfigValueDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = dataSourceSchema.Schema{
		MarkdownDescription: "This data source allows you to get a specific cluster configuration value for a given section.",
		Attributes: map[string]dataSourceSchema.Attribute{
			"name": dataSourceSchema.StringAttribute{
				MarkdownDescription: "The name of the configuration option",
				Required:            true,
			},
			"section": dataSourceSchema.StringAttribute{
				MarkdownDescription: "The configuration section (e.g., 'global', 'mon', 'osd', 'osd.1')",
				Required:            true,
			},
			"value": dataSourceSchema.StringAttribute{
				MarkdownDescription: "The configuration value for the specified section",
				Computed:            true,
			},
		},
	}
}

func (d *ConfigValueDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *ConfigValueDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data ConfigValueDataSourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	name := data.Name.ValueString()
	section := data.Section.ValueString()

	config, err := d.client.ClusterGetConf(ctx, name)
	if err != nil {
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to get cluster configuration '%s' from Ceph API: %s", name, err),
		)
		return
	}

	found := false
	for _, v := range config.Value {
		if v.Section == section {
			data.Value = types.StringValue(v.Value)
			found = true
			break
		}
	}

	if !found {
		resp.Diagnostics.AddError(
			"Configuration Not Found",
			fmt.Sprintf("Configuration '%s' is not set for section '%s'", name, section),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
