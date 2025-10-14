package main

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	dataSourceSchema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSource = &AuthDataSource{}

func newAuthDataSource() datasource.DataSource {
	return &AuthDataSource{}
}

type AuthDataSource struct {
	client *CephAPIClient
}

type AuthDataSourceModel struct {
	Entity  types.String `tfsdk:"entity"`
	Caps    types.Map    `tfsdk:"caps"`
	Key     types.String `tfsdk:"key"`
	Keyring types.String `tfsdk:"keyring"`
}

func (d *AuthDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_auth"
}

func (d *AuthDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = dataSourceSchema.Schema{
		MarkdownDescription: "This data source allows you to get information about a ceph client.",
		Attributes: map[string]dataSourceSchema.Attribute{
			"entity": dataSourceSchema.StringAttribute{
				MarkdownDescription: "The entity name (i.e.: client.admin)",
				Required:            true,
			},
			"caps": dataSourceSchema.MapAttribute{
				ElementType:         types.StringType,
				MarkdownDescription: "The caps of the entity",
				Computed:            true,
			},
			"key": dataSourceSchema.StringAttribute{
				MarkdownDescription: "The cephx key of the entity",
				Computed:            true,
				Sensitive:           true,
			},
			"keyring": dataSourceSchema.StringAttribute{
				MarkdownDescription: "The complete cephx keyring as JSON",
				Computed:            true,
				Sensitive:           true,
			},
		},
	}
}

func (d *AuthDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	client, ok := req.ProviderData.(*CephAPIClient)

	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			fmt.Sprintf("Expected *CephClient, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)

		return
	}

	d.client = client
}

func (d *AuthDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data AuthDataSourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	entity := data.Entity.ValueString()
	keyringRaw, err := d.client.ClusterExportUser(ctx, entity)
	if err != nil {
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to export user from Ceph API: %s", err),
		)
		return
	}

	keyringUsers, err := parseCephKeyring(keyringRaw)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to parse keyring data",
			fmt.Sprintf("Unable to parse keyring data: %s", err),
		)
		return
	} else if len(keyringUsers) == 0 {
		resp.Diagnostics.AddError(
			"Empty keyring data",
			fmt.Sprintf("Ceph export returned no users for entity %s", entity),
		)
		return
	} else if len(keyringUsers) > 1 {
		resp.Diagnostics.AddWarning(
			"Ceph export returned multiple users",
			fmt.Sprintf("Ceph export returned multiple users: %s", keyringRaw),
		)
	}
	keyringUser := keyringUsers[0]

	data.Caps = cephCapsToMapValue(ctx, keyringUser.Caps, &resp.Diagnostics)
	data.Key = types.StringValue(keyringUser.Key)
	data.Keyring = types.StringValue(keyringRaw)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
