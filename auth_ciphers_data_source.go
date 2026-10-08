package main

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	dataSourceSchema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/josh/terraform-provider-ceph/internal/restapi"
)

var _ datasource.DataSource = &AuthCiphersDataSource{}

func newAuthCiphersDataSource() datasource.DataSource {
	return &AuthCiphersDataSource{}
}

type AuthCiphersDataSource struct {
	client *restapi.Client
}

type AuthCiphersDataSourceModel struct {
	AllowedCiphers  types.List   `tfsdk:"allowed_ciphers"`
	PreferredCipher types.String `tfsdk:"preferred_cipher"`
	ServiceCipher   types.String `tfsdk:"service_cipher"`
}

func (d *AuthCiphersDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_auth_ciphers"
}

func (d *AuthCiphersDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = dataSourceSchema.Schema{
		MarkdownDescription: "This data source reads the cephx cipher settings from the monitor map. All attributes are null on Ceph releases before 19.2.6/20.2.4, which have no cipher settings.",
		Attributes: map[string]dataSourceSchema.Attribute{
			"allowed_ciphers": dataSourceSchema.ListAttribute{
				ElementType:         types.StringType,
				MarkdownDescription: "Key types the monitors accept for authentication (`auth_allowed_ciphers`)",
				Computed:            true,
			},
			"preferred_cipher": dataSourceSchema.StringAttribute{
				MarkdownDescription: "Key type Ceph uses for new keys when none is requested (`auth_preferred_cipher`)",
				Computed:            true,
			},
			"service_cipher": dataSourceSchema.StringAttribute{
				MarkdownDescription: "Key type used for rotating service keys (`auth_service_cipher`)",
				Computed:            true,
			},
		},
	}
}

func (d *AuthCiphersDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *AuthCiphersDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data AuthCiphersDataSourceModel

	monitor, err := d.client.GetMonitor(ctx)
	if err != nil {
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to get monitor status from Ceph API: %s", err),
		)
		return
	}
	monmap := monitor.MonStatus.Monmap

	data.AllowedCiphers = types.ListNull(types.StringType)
	if monmap.AuthAllowedCiphers != nil {
		names := make([]string, 0, len(monmap.AuthAllowedCiphers))
		for _, cipher := range monmap.AuthAllowedCiphers {
			names = append(names, cipher.Name)
		}
		list, diags := types.ListValueFrom(ctx, types.StringType, names)
		resp.Diagnostics.Append(diags...)
		data.AllowedCiphers = list
	}
	data.PreferredCipher = cipherName(monmap.AuthPreferredCipher)
	data.ServiceCipher = cipherName(monmap.AuthServiceCipher)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func cipherName(cipher *restapi.MonitorCipher) types.String {
	if cipher == nil {
		return types.StringNull()
	}
	return types.StringValue(cipher.Name)
}
